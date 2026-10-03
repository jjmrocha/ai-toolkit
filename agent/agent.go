package agent

import (
	"context"
	"slices"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/skills"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/fn"
	"github.com/jjmrocha/go-algo/token"
)

type modelInterface interface {
	Chat(ctx context.Context, messages []llm.Message, tools []llm.Tool) (*llm.AssistantMessage, error)
	ModelInfo(ctx context.Context) (*llm.ModelInfo, error)
	AvailableModels() []string
	ChangeModel(model string) error
	Effort() llm.Effort
	ChangeEffort(e llm.Effort) error
}

// Agent runs a tool-calling chat loop against an LLM and holds one session's
// conversation. It is not safe for concurrent use: serialize calls to its
// methods.
type Agent struct {
	config           Config
	llm              modelInterface
	toolBox          *tools.ToolBox
	skills           *skills.Collection
	fb               Feedback
	messages         []llm.Message
	sessionID        string
	compactThreshold int
	modelInfo        *llm.ModelInfo
}

// New creates an [Agent] for an [llm.LLM], with silent [Feedback]; use
// [Agent.SetFeedback] with [NewStdoutFeedback] to print events. It returns
// [ErrNoLLM] when llm is nil and [ErrInvalidThreshold] when
// Config.CompactionThresholdPercent is outside 0 to 100.
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

// StartSession starts a new conversation, discarding any previous one, and must
// be called before [Agent.Process]. SessionConfig.Prompt becomes the system
// message and survives [Agent.ResetSession]. SessionConfig.ToolBox holds the
// tools the model may call. Skills in SessionConfig.Skills register their tools
// in that ToolBox and append their catalog to the system message, until
// [Agent.Close] or the next session. SessionConfig.Messages, if set, resumes a
// saved conversation after the system message. The session gets a new id, or
// SessionConfig.ID if set; see [Agent.SessionID]. It fires
// [Feedback.SessionResumed] when it restores at least one message, and
// [Feedback.SessionStarted] otherwise.
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

	restored := fn.Filter(cfg.Messages, func(m llm.Message) bool {
		return m.Role() != llm.SystemRole
	})
	a.messages = append(a.messages, restored...)

	a.sessionID = cfg.ID
	if a.sessionID == "" {
		a.sessionID = token.New()
	}

	if len(restored) > 0 {
		a.fb.SessionResumed(a.sessionID)
		return
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

// ResetSession drops every turn after the system message and gives the session
// a new id. It returns [ErrNoSession] if no session has been started.
func (a *Agent) ResetSession() error {
	if len(a.messages) == 0 {
		return ErrNoSession
	}

	a.messages = a.messages[:1]
	a.sessionID = token.New()
	a.fb.SessionReset()
	return nil
}

// Close ends the session and removes the skill tools it registered in the
// session's [tools.ToolBox]. After Close, [Agent.Process] returns
// [ErrNoSession] until a new session starts.
func (a *Agent) Close() {
	a.unregisterSkills()
	a.toolBox = nil
	a.messages = nil
	a.sessionID = ""
	a.fb.SessionClosed()
}

// Messages returns a copy of the conversation as the model sees it: the system
// message (the prompt plus any skill catalog), then every later turn, with
// turns folded by [Agent.CompactContext] shown as their summary. Pass it to
// SessionConfig.Messages to resume the conversation later. It returns nil when
// there is no session.
func (a *Agent) Messages() []llm.Message {
	return slices.Clone(a.messages)
}

// SessionID returns the id of the current session. [Agent.StartSession] sets it,
// to SessionConfig.ID or a new one, and [Agent.ResetSession] replaces it with a
// new one. It returns "" when there is no session.
func (a *Agent) SessionID() string {
	return a.sessionID
}

// SetFeedback replaces the agent's [Feedback], for example with a chat UI's own.
// A nil fb is ignored. Do not call it while [Agent.Process] is running.
func (a *Agent) SetFeedback(fb Feedback) {
	if fb == nil {
		return
	}
	a.fb = fb
}

// Process runs one round: it appends userInput, if not empty, and calls the
// model again and again, running every tool it asks for and feeding back the
// results, until it replies without asking for tools. It returns that reply as
// a [Response] with token usage and timing in [Metadata]. A failing tool is
// reported to the model as its error text, so the model can recover and the
// round goes on.
//
// The tools offered to the model are read from the session's ToolBox once,
// before the first model call, and stay fixed for the round. A tool registered
// during the round, by an MCP server changing its tool list, say, is offered
// from the next Process call. A tool removed during the round is still offered
// until then, and calling it fails with [tools.ErrToolNotFound], which the
// model sees as tool error text.
//
// The first round also asks for the model's context window (see
// [llm.LLM.ModelInfo]) and caches it. Once a completed turn goes past
// Config.CompactionThresholdPercent of that window, the older turns are
// summarized before the next round.
//
// Process returns [ErrNoSession] if no session has been started,
// [ErrMaxIterations] if it reaches Config.MaxIterations, or the model's error.
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

		if response.Content != "" {
			a.fb.InterimTextReceived(response.Content)
		}

		a.fb.TokensUsed(response.Stats.TotalTokens)

		for _, call := range response.ToolCalls {
			a.fb.ToolCalled(call.Name, call.Arguments)

			t0 := time.Now()
			result, err := a.toolBox.Execute(ctx, call)
			elapsed := time.Since(t0)
			toolDuration += elapsed
			callCount++

			a.fb.ToolReturned(call.Name, toolResult(result), err, elapsed)

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

// toolResult is the content of a successful call's message, and empty for a
// call that failed, so [Feedback.ToolReturned] never reports both a result and
// an error.
func toolResult(msg *llm.ToolMessage) string {
	if msg == nil {
		return ""
	}

	return msg.Content
}

func (a *Agent) compactIfNeeded(ctx context.Context, lastTotalTokens int) {
	a.loadModelLimits(ctx)

	if a.compactThreshold != 0 && lastTotalTokens > a.compactThreshold {
		a.CompactContext(ctx)
	}
}

// ModelInfo returns the model the agent uses: provider, name, context window
// and reasoning effort. It fetches the context window if needed, so it works
// before the first turn. It returns nil if the client cannot provide it.
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

// AvailableModels returns the models [Agent.ChangeModel] accepts. It always
// includes the active model.
func (a *Agent) AvailableModels() []string {
	return a.llm.AvailableModels()
}

// ChangeModel switches the agent to model, which must be in
// [Agent.AvailableModels]. The context window is fetched again on the next
// turn. On failure it returns the client's error and keeps the current model.
func (a *Agent) ChangeModel(model string) error {
	if err := a.llm.ChangeModel(model); err != nil {
		return err
	}

	a.modelInfo = nil
	a.compactThreshold = 0

	return nil
}

// ChangeEffort sets the reasoning effort for later turns. On failure it returns
// the client's error and keeps the current effort.
func (a *Agent) ChangeEffort(e llm.Effort) error {
	return a.llm.ChangeEffort(e)
}

// CompactContext summarizes the conversation, keeping the system message and
// the last turn as they are. [Agent.Process] calls it once a completed turn
// goes past Config.CompactionThresholdPercent of the context window; call it
// yourself to cut token cost sooner. It does nothing if there is nothing to
// summarize or the model fails to summarize.
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
