package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLLM is a model double. Each Chat call returns the next reply in replies
// (or chatErr); ModelInfo returns info (or infoErr).
type fakeLLM struct {
	replies   []*llm.AssistantMessage
	chatErr   error
	info      *llm.ModelInfo
	infoErr   error
	chatCalls int
	models    []string
	current   string
	changeErr error
	effort    llm.Effort
}

func (f *fakeLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.Tool) (*llm.AssistantMessage, error) {
	if f.chatErr != nil {
		return nil, f.chatErr
	}
	reply := f.replies[f.chatCalls]
	f.chatCalls++
	return reply, nil
}

func (f *fakeLLM) ModelInfo(context.Context) (*llm.ModelInfo, error) {
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	return f.info, nil
}

func (f *fakeLLM) AvailableModels() []string { return f.models }

func (f *fakeLLM) ChangeModel(model string) error {
	if f.changeErr != nil {
		return f.changeErr
	}
	f.current = model
	return nil
}

func (f *fakeLLM) Effort() llm.Effort { return f.effort }

func (f *fakeLLM) ChangeEffort(e llm.Effort) { f.effort = e }

// agentWithLLM builds an Agent backed by a model double, bypassing the
// constructor's *llm.LLM requirement.
func agentWithLLM(m modelInterface, tb *tools.ToolBox, fb Feedback, cfg Config) *Agent {
	if tb == nil {
		tb = tools.NewToolBox()
	}
	return &Agent{config: cfg, llm: m, toolBox: tb, fb: fb}
}

// recordingFeedback is a Feedback double that records the lifecycle events it
// receives, in order, so tests can assert the observable side effects of the
// session methods.
type recordingFeedback struct {
	events []string
	tools  []string
}

func (f *recordingFeedback) ToolCalled(toolName string) {
	f.tools = append(f.tools, toolName)
	f.events = append(f.events, "ToolCalled")
}
func (f *recordingFeedback) ContextCompacted() { f.events = append(f.events, "ContextCompacted") }
func (f *recordingFeedback) SessionReset()     { f.events = append(f.events, "SessionReset") }
func (f *recordingFeedback) SessionStarted()   { f.events = append(f.events, "SessionStarted") }
func (f *recordingFeedback) SessionClosed()    { f.events = append(f.events, "SessionClosed") }

// testLLM builds a real *llm.LLM. Construction performs no I/O, so it is safe in
// unit tests; only Chat/ModelInfo would reach the network.
func testLLM(t *testing.T) *llm.LLM {
	t.Helper()
	client, err := llm.New(llm.Config{Provider: llm.ProviderOllama, Model: "test-model"})
	require.NoError(t, err)
	return client
}

func newTestAgent(t *testing.T, cfg Config, fb Feedback) *Agent {
	t.Helper()
	agt, err := New(cfg, testLLM(t), nil)
	require.NoError(t, err)
	agt.SetFeedback(fb)
	return agt
}

func TestNew(t *testing.T) {
	t.Run("returns an agent for a valid configuration", func(t *testing.T) {
		// when
		result, err := New(Config{}, testLLM(t), nil)
		// then
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("propagates ErrNoLLM when llm is nil", func(t *testing.T) {
		// when
		result, err := New(Config{}, nil, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrNoLLM)
	})

	t.Run("returns ErrInvalidThreshold when the percent is negative", func(t *testing.T) {
		// when
		result, err := New(Config{CompactionThresholdPercent: -1}, testLLM(t), nil)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrInvalidThreshold)
	})

	t.Run("returns ErrInvalidThreshold when the percent exceeds one hundred", func(t *testing.T) {
		// when
		result, err := New(Config{CompactionThresholdPercent: 101}, testLLM(t), nil)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrInvalidThreshold)
	})

	t.Run("succeeds for percents within range", func(t *testing.T) {
		for _, pct := range []int{0, 50, 100} {
			// when
			result, err := New(Config{CompactionThresholdPercent: pct}, testLLM(t), nil)
			// then
			require.NoError(t, err, "pct=%d", pct)
			assert.NotNil(t, result, "pct=%d", pct)
		}
	})
}

func TestStartSession(t *testing.T) {
	t.Run("fires the SessionStarted event", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := newTestAgent(t, Config{}, fb)
		// when
		agt.StartSession("be terse")
		// then
		assert.Equal(t, []string{"SessionStarted"}, fb.events)
	})
}

func TestResetSession(t *testing.T) {
	t.Run("returns ErrNoSession before a session has started", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := newTestAgent(t, Config{}, fb)
		// when
		err := agt.ResetSession()
		// then
		assert.ErrorIs(t, err, ErrNoSession)
		assert.Empty(t, fb.events) // SessionReset not fired
	})

	t.Run("succeeds and fires SessionReset after a session has started", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := newTestAgent(t, Config{}, fb)
		agt.StartSession("sys")
		// when
		err := agt.ResetSession()
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"SessionStarted", "SessionReset"}, fb.events)
	})
}

func TestClose(t *testing.T) {
	t.Run("fires the SessionClosed event", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := newTestAgent(t, Config{}, fb)
		agt.StartSession("sys")
		// when
		agt.Close()
		// then
		assert.Equal(t, []string{"SessionStarted", "SessionClosed"}, fb.events)
	})
}

func TestSetFeedback(t *testing.T) {
	t.Run("routes later events to the new sink, not the old one", func(t *testing.T) {
		// given
		old := &recordingFeedback{}
		replacement := &recordingFeedback{}
		agt := newTestAgent(t, Config{}, old)
		// when
		agt.SetFeedback(replacement)
		agt.StartSession("sys")
		// then
		assert.Empty(t, old.events)
		assert.Equal(t, []string{"SessionStarted"}, replacement.events)
	})

	t.Run("keeps the existing sink when fb is nil", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := newTestAgent(t, Config{}, fb)
		// when
		agt.SetFeedback(nil)
		agt.StartSession("sys")
		// then
		assert.Equal(t, []string{"SessionStarted"}, fb.events)
	})
}

func TestProcess(t *testing.T) {
	t.Run("returns ErrNoSession when no session has started", func(t *testing.T) {
		// given
		agt := newTestAgent(t, Config{}, &recordingFeedback{})
		// when
		result, err := agt.Process(context.Background(), "hi")
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrNoSession)
	})

	t.Run("returns ErrNoSession after the session is closed", func(t *testing.T) {
		// given
		agt := newTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession("sys")
		agt.Close()
		// when
		result, err := agt.Process(context.Background(), "hi")
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrNoSession)
	})

	t.Run("returns the final reply with token usage", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{Content: "hello", Stats: llm.Stats{PromptTokens: 10, OutputTokens: 5, TotalTokens: 15}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, nil, &recordingFeedback{}, Config{})
		agt.StartSession("sys")
		// when
		result, err := agt.Process(context.Background(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "hello", result.Content)
		assert.Equal(t, 15, result.Metadata.TotalTokens)
		assert.Equal(t, 0, result.Metadata.Iterations)
	})

	t.Run("runs requested tools and feeds the results back", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo"}}},
				{Content: "done", Stats: llm.Stats{TotalTokens: 20}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		tb.AddTool(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil })
		agt := agentWithLLM(fake, tb, fb, Config{})
		agt.StartSession("sys")
		// when
		result, err := agt.Process(context.Background(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "done", result.Content)
		assert.Equal(t, 1, result.Metadata.ToolCalls)
		assert.Equal(t, 1, result.Metadata.Iterations)
		assert.Equal(t, []string{"echo"}, fb.tools)
	})

	t.Run("returns ErrMaxIterations when the iteration cap is reached", func(t *testing.T) {
		// given: the model never stops requesting tools
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{{Name: "echo"}}},
				{ToolCalls: []llm.ToolCall{{Name: "echo"}}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		tb.AddTool(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil })
		agt := agentWithLLM(fake, tb, &recordingFeedback{}, Config{MaxIterations: 2})
		agt.StartSession("sys")
		// when
		result, err := agt.Process(context.Background(), "hi")
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrMaxIterations)
	})

	t.Run("propagates an error from the model", func(t *testing.T) {
		// given
		fake := &fakeLLM{chatErr: errors.New("boom")}
		agt := agentWithLLM(fake, nil, &recordingFeedback{}, Config{})
		agt.StartSession("sys")
		// when
		result, err := agt.Process(context.Background(), "hi")
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("compacts older turns once the threshold is crossed", func(t *testing.T) {
		// given: two small turns, then a third that trips 90% of the 1000-token
		// window, plus a fourth Chat call that is the summarization inside compaction.
		// defaultKeepRecentTurns is 2, so three turns are needed for an older turn to exist.
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{Content: "a1", Stats: llm.Stats{TotalTokens: 100}},
				{Content: "a2", Stats: llm.Stats{TotalTokens: 100}},
				{Content: "a3", Stats: llm.Stats{TotalTokens: 950}},
				{Content: "SUMMARY"},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, nil, fb, Config{CompactionThresholdPercent: 90})
		agt.StartSession("sys")
		// when: three turns build enough history for compaction to have an older turn
		_, err1 := agt.Process(context.Background(), "u1")
		_, err2 := agt.Process(context.Background(), "u2")
		_, err3 := agt.Process(context.Background(), "u3")
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		require.NoError(t, err3)
		assert.Contains(t, fb.events, "ContextCompacted")
		assert.Equal(t, 4, fake.chatCalls) // three turns plus one summarization
	})

	t.Run("feeds a failing tool's error back to the model and continues", func(t *testing.T) {
		// given: the tool errors on the first turn; the model then answers.
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "boom"}}},
				{Content: "recovered", Stats: llm.Stats{TotalTokens: 5}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		tb.AddTool(llm.Tool{Name: "boom"}, func(context.Context, map[string]any) (string, error) {
			return "", errors.New("kaboom")
		})
		agt := agentWithLLM(fake, tb, fb, Config{})
		agt.StartSession("sys")
		// when
		result, err := agt.Process(context.Background(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "recovered", result.Content)
		assert.Equal(t, 1, result.Metadata.ToolCalls)
		// the failure is fed back as a tool message so the model can recover
		var toolMsg *llm.ToolMessage
		for i := range agt.messages {
			if m, ok := agt.messages[i].(llm.ToolMessage); ok {
				toolMsg = &m
			}
		}
		require.NotNil(t, toolMsg)
		assert.Equal(t, "c1", toolMsg.ToolCallID)
		assert.Contains(t, toolMsg.Content, "kaboom")
	})

	t.Run("skips compaction when the model info is unavailable", func(t *testing.T) {
		// given: ModelInfo fails, so limits stay zero and compaction is skipped
		// even though the turn reports a huge token count.
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi", Stats: llm.Stats{TotalTokens: 9999}}},
			infoErr: errors.New("no info"),
		}
		agt := agentWithLLM(fake, nil, &recordingFeedback{}, Config{CompactionThresholdPercent: 1})
		agt.StartSession("sys")
		// when
		result, err := agt.Process(context.Background(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "hi", result.Content)
		assert.Nil(t, agt.modelInfo)
		assert.Zero(t, agt.compactThreshold)
	})
}

func TestAvailableModels(t *testing.T) {
	t.Run("returns the underlying client's model list", func(t *testing.T) {
		// given
		fake := &fakeLLM{models: []string{"m1", "m2"}}
		agt := agentWithLLM(fake, nil, &recordingFeedback{}, Config{})
		// when
		result := agt.AvailableModels()
		// then
		assert.Equal(t, []string{"m1", "m2"}, result)
	})
}

func TestChangeModel(t *testing.T) {
	t.Run("updates the current model and resets the context window", func(t *testing.T) {
		// given: an agent whose limits were loaded for a 1000-token model
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi", Stats: llm.Stats{TotalTokens: 1}}},
			info:    &llm.ModelInfo{Name: "old", ContextSize: 1000},
			models:  []string{"m2"},
		}
		agt := agentWithLLM(fake, nil, &recordingFeedback{}, Config{})
		agt.StartSession("sys")
		_, err := agt.Process(context.Background(), "hi")
		require.NoError(t, err)
		require.Equal(t, 1000, agt.modelInfo.ContextSize)
		// when
		err = agt.ChangeModel("m2")
		// then
		require.NoError(t, err)
		assert.Equal(t, "m2", fake.current)
		assert.Nil(t, agt.modelInfo)         // forced reload on next turn
		assert.Zero(t, agt.compactThreshold) // forced reload on next turn
	})

	t.Run("propagates the client error and keeps the current model", func(t *testing.T) {
		// given
		fake := &fakeLLM{changeErr: errors.New("boom"), current: "old"}
		agt := agentWithLLM(fake, nil, &recordingFeedback{}, Config{})
		// when
		err := agt.ChangeModel("m2")
		// then
		assert.ErrorContains(t, err, "boom")
		assert.Equal(t, "old", fake.current)
	})
}

func TestModelInfo(t *testing.T) {
	t.Run("reports the active model's name, context window, and effort", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			info:   &llm.ModelInfo{Name: "m1", ContextSize: 2000},
			effort: llm.EffortMedium,
		}
		agt := agentWithLLM(fake, nil, &recordingFeedback{}, Config{})
		// when
		result := agt.ModelInfo(context.Background())
		// then
		require.NotNil(t, result)
		assert.Equal(t, "m1", result.ModelName)
		assert.Equal(t, 2000, result.ModelContextSize)
		assert.Equal(t, llm.EffortMedium, result.Effort)
	})

	t.Run("returns nil when the model info cannot be fetched", func(t *testing.T) {
		// given: called before any turn, with a client that cannot report info
		fake := &fakeLLM{infoErr: errors.New("no info")}
		agt := agentWithLLM(fake, nil, &recordingFeedback{}, Config{})
		// when
		result := agt.ModelInfo(context.Background())
		// then
		assert.Nil(t, result)
	})
}

func TestChangeEffort(t *testing.T) {
	t.Run("sets the effort on the underlying client", func(t *testing.T) {
		// given
		fake := &fakeLLM{}
		agt := agentWithLLM(fake, nil, &recordingFeedback{}, Config{})
		// when
		agt.ChangeEffort(llm.EffortMax)
		// then
		assert.Equal(t, llm.EffortMax, fake.effort)
	})
}

func TestCompactContext(t *testing.T) {
	t.Run("is a no-op when there is no older turn to summarize", func(t *testing.T) {
		// given: a single completed turn — nothing older than the kept window
		fb := &recordingFeedback{}
		fake := &fakeLLM{}
		agt := agentWithLLM(fake, nil, fb, Config{})
		agt.messages = []llm.Message{
			llm.SystemMessage{Content: "sys"},
			llm.UserMessage{Content: "u1"},
			llm.AssistantMessage{Content: "a1"},
		}
		before := len(agt.messages)
		// when
		agt.CompactContext(context.Background())
		// then
		assert.Len(t, agt.messages, before) // unchanged
		assert.Zero(t, fake.chatCalls)      // no summarization attempted
		assert.NotContains(t, fb.events, "ContextCompacted")
	})

	t.Run("leaves the conversation unchanged when summarization fails", func(t *testing.T) {
		// given: enough turns that an older one exists, but the summarizing Chat errors
		fb := &recordingFeedback{}
		fake := &fakeLLM{chatErr: errors.New("summary boom")}
		agt := agentWithLLM(fake, nil, fb, Config{})
		agt.messages = []llm.Message{
			llm.SystemMessage{Content: "sys"},
			llm.UserMessage{Content: "u1"}, llm.AssistantMessage{Content: "a1"},
			llm.UserMessage{Content: "u2"}, llm.AssistantMessage{Content: "a2"},
			llm.UserMessage{Content: "u3"}, llm.AssistantMessage{Content: "a3"},
		}
		snapshot := append([]llm.Message(nil), agt.messages...)
		// when
		agt.CompactContext(context.Background())
		// then
		assert.Equal(t, snapshot, agt.messages) // unchanged on failure
		assert.NotContains(t, fb.events, "ContextCompacted")
	})
}
