package agent

import (
	"context"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/skills"
	"github.com/jjmrocha/ai-toolkit/tools"
)

type modelInterface interface {
	Chat(ctx context.Context, messages []llm.Message, tools []llm.Tool) (*llm.AssistantMessage, error)
	ModelInfo(ctx context.Context) (*llm.ModelInfo, error)
	AvailableModels() []string
	ChangeModel(model string) error
	Effort() llm.Effort
	ChangeEffort(e llm.Effort) error
}

// Agent runs a tool-calling chat loop against an LLM, holding the conversation
// state for a single session. It is not safe for concurrent use; serialize
// calls to Process, StartSession, ResetSession, and Close.
type Agent struct {
	config           Config
	llm              modelInterface
	toolBox          *tools.ToolBox
	skills           *skills.Collection
	fb               Feedback
	messages         []llm.Message
	compactThreshold int
	modelInfo        *llm.ModelInfo
}

// New creates an [Agent] from cfg and an [llm.LLM], using a silent default
// [Feedback]; install [NewStdoutFeedback] with [Agent.SetFeedback] to print
// lifecycle events. It returns [ErrNoLLM] when llm is nil and
// [ErrInvalidThreshold] when Config.CompactionThresholdPercent is outside
// 0–100.
func New(cfg Config, llm *llm.LLM) (*Agent, error) {
	if llm == nil {
		return nil, ErrNoLLM
	}

	if cfg.CompactionThresholdPercent < 0 || cfg.CompactionThresholdPercent > 100 {
		return nil, ErrInvalidThreshold
	}

	feedback := &nullFeedback{}

	return &Agent{
		config: cfg,
		llm:    llm,
		fb:     feedback,
	}, nil
}

// StartSession begins a new conversation, discarding any previous one.
// SessionConfig.Prompt becomes the session's system message and is preserved
// across [Agent.ResetSession]; SessionConfig.ToolBox holds the tools the model
// may call until the session ends. When SessionConfig.Skills carries at least
// one skill, its tools are registered in that ToolBox and its catalog is
// appended to the system message, both until [Agent.Close] or the next session.
// It must be called before [Agent.Process].
func (a *Agent) StartSession(cfg SessionConfig) {
	a.unregisterSkills()

	toolBox := cfg.ToolBox
	if toolBox == nil {
		toolBox = tools.NewToolBox()
	}

	a.toolBox = toolBox
	a.skills = cfg.Skills
	prompt := cfg.Prompt

	if a.skills != nil {
		if catalog := a.skills.Catalog(); catalog != "" {
			a.skills.RegisterTools(a.toolBox)
			prompt += "\n\n" + catalog
		}
	}

	a.messages = []llm.Message{
		llm.SystemMessage{
			Content: prompt,
		},
	}

	a.fb.SessionStarted()
}

func (a *Agent) unregisterSkills() {
	if a.skills == nil || a.toolBox == nil {
		return
	}

	a.skills.UnregisterTools(a.toolBox)
	a.skills = nil
}

// ResetSession clears the conversation back to its system message, keeping the
// prompt from [Agent.StartSession] but discarding all later turns. It returns
// [ErrNoSession] if no session has been started.
func (a *Agent) ResetSession() error {
	if len(a.messages) == 0 {
		return ErrNoSession
	}

	a.messages = a.messages[:1]
	a.fb.SessionReset()
	return nil
}

// Close ends the agent's session and releases its conversation state, removing
// any skill tools it registered in the session's [tools.ToolBox]. After Close,
// [Agent.Process] returns [ErrNoSession] until a new session is started.
func (a *Agent) Close() {
	a.unregisterSkills()
	a.toolBox = nil
	a.messages = nil
	a.fb.SessionClosed()
}

// SetFeedback replaces the agent's lifecycle event sink, letting a caller — such
// as a chat UI — install its own [Feedback] after construction. A nil fb is
// ignored, keeping the current sink. Like the rest of [Agent], it is not safe
// for concurrent use; do not call it while a [Agent.Process] is in flight.
func (a *Agent) SetFeedback(fb Feedback) {
	if fb == nil {
		return
	}
	a.fb = fb
}

// Process runs one round of the conversation: it appends userInput (when
// non-empty) and repeatedly calls the model, executing every tool the model
// requests and feeding the results back, until the model replies without
// requesting tools. That final reply is returned as a [Response] together with
// token usage and timing [Metadata]. A failing tool is reported to the model as
// its error text so the model can recover rather than aborting the round.
//
// On the first round it also queries the model's context window (see
// [llm.LLM.ModelInfo]) to size the compaction threshold; the result is cached
// for the agent's lifetime. Once a completed turn crosses
// Config.CompactionThresholdPercent of that window, the older turns are
// summarized before the next round.
//
// Process returns [ErrNoSession] if no session has been started,
// [ErrMaxIterations] if Config.MaxIterations is reached first, or any error from
// the model. The context controls cancellation and deadline.
func (a *Agent) Process(ctx context.Context, userInput string) (*Response, error) {
	if len(a.messages) == 0 {
		return nil, ErrNoSession
	}

	if userInput != "" {
		a.messages = append(a.messages, llm.UserMessage{Content: userInput})
	}

	toolList := a.toolBox.Tools()

	var (
		callCount    int
		llmDuration  time.Duration
		toolDuration time.Duration
		iteration    int
	)

	for {
		if a.config.MaxIterations != 0 && iteration >= a.config.MaxIterations {
			return nil, ErrMaxIterations
		}

		t0 := time.Now()
		response, err := a.llm.Chat(ctx, a.messages, toolList)
		llmDuration += time.Since(t0)
		if err != nil {
			return nil, err
		}

		a.messages = append(a.messages, *response)

		if len(response.ToolCalls) == 0 {
			a.compactIfNeeded(ctx, response.Stats.TotalTokens)

			return &Response{
				Content: response.Content,
				Metadata: Metadata{
					Iterations:   iteration,
					PromptTokens: response.Stats.PromptTokens,
					OutputTokens: response.Stats.OutputTokens,
					TotalTokens:  response.Stats.TotalTokens,
					ToolCalls:    callCount,
					StopReason:   response.StopReason,
					LLMDuration:  llmDuration,
					ToolDuration: toolDuration,
				},
			}, nil
		}

		for _, call := range response.ToolCalls {
			a.fb.ToolCalled(call.Name)

			t0 := time.Now()
			result, err := a.toolBox.Execute(ctx, call)
			toolDuration += time.Since(t0)
			callCount++

			if err != nil {
				a.messages = append(a.messages, llm.ToolMessage{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Content:    err.Error(),
				})
				continue
			}

			a.messages = append(a.messages, *result)
		}

		iteration++
	}
}

func (a *Agent) compactIfNeeded(ctx context.Context, lastTotalTokens int) {
	a.loadModelLimits(ctx)

	if a.compactThreshold != 0 && lastTotalTokens > a.compactThreshold {
		a.CompactContext(ctx)
	}
}

// ModelInfo reports the model the agent is currently using — its provider,
// name, context window, and reasoning effort. It resolves the model's context
// window on demand, so it is safe to call before the first turn; it returns nil
// when that information cannot be fetched from the underlying client.
func (a *Agent) ModelInfo(ctx context.Context) *ModelInfo {
	a.loadModelLimits(ctx)
	if a.modelInfo == nil {
		return nil
	}

	return &ModelInfo{
		Provider:         a.modelInfo.Provider,
		ModelName:        a.modelInfo.Name,
		ModelContextSize: a.modelInfo.ContextSize,
		Effort:           a.llm.Effort(),
	}
}

// AvailableModels returns the model identifiers the agent can switch to via
// [Agent.ChangeModel]. The active model is always included, so the result is
// never empty.
func (a *Agent) AvailableModels() []string {
	return a.llm.AvailableModels()
}

// ChangeModel switches the agent to model, which must be one of
// [Agent.AvailableModels]. On success the context window is re-derived on the
// next turn; it propagates the underlying client's error on failure, leaving the
// current model in place.
func (a *Agent) ChangeModel(model string) error {
	if err := a.llm.ChangeModel(model); err != nil {
		return err
	}

	a.modelInfo = nil
	a.compactThreshold = 0

	return nil
}

// ChangeEffort sets the reasoning effort applied to subsequent turns. It
// propagates the underlying client's error, leaving the current effort in
// place.
func (a *Agent) ChangeEffort(e llm.Effort) error {
	return a.llm.ChangeEffort(e)
}

// CompactContext summarizes the conversation up to the most recent turn, keeping
// the system message and the last turn intact. It is called automatically when
// a completed turn crosses Config.CompactionThresholdPercent of the model's
// context window, but can also be called manually to reduce memory usage or
// token cost. It does nothing if there are no turns to summarize or if the
// model fails to produce a summary.
func (a *Agent) CompactContext(ctx context.Context) {
	keepFrom := indexOfTheBeginningOfTurnToKeep(a.messages)
	if keepFrom <= 1 {
		return
	}

	older := a.messages[1:keepFrom]

	reply, err := a.llm.Chat(ctx, []llm.Message{
		llm.SystemMessage{Content: summarySystemPrompt},
		llm.UserMessage{Content: renderConversation(older)},
	}, nil)
	if err != nil {
		a.fb.ContextCompactionFailed()
		return
	}

	compacted := make([]llm.Message, 0, 2+len(a.messages)-keepFrom)
	compacted = append(compacted, a.messages[0])
	compacted = append(compacted, llm.UserMessage{Content: summaryPrefix + reply.Content})
	compacted = append(compacted, a.messages[keepFrom:]...)

	a.messages = compacted
	a.fb.ContextCompacted()
}

func (a *Agent) loadModelLimits(ctx context.Context) {
	if a.modelInfo != nil {
		return
	}

	info, err := a.llm.ModelInfo(ctx)
	if err != nil {
		a.fb.ModelInfoUnavailable()
		return
	}

	a.modelInfo = info
	a.compactThreshold = compactionThreshold(info.ContextSize, a.config.CompactionThresholdPercent)
}
