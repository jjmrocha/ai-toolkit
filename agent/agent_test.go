package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/skills"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLLM struct {
	replies   []*llm.AssistantMessage
	chatErr   error
	info      *llm.ModelInfo
	infoErr   error
	chatCalls int
	calls     [][]llm.Message
	models    []string
	current   string
	changeErr error
	effort    llm.Effort
	toolLists [][]llm.Tool
}

func (f *fakeLLM) Chat(_ context.Context, messages []llm.Message, toolList []llm.Tool) (*llm.AssistantMessage, error) {
	if f.chatErr != nil {
		return nil, f.chatErr
	}
	f.calls = append(f.calls, messages)
	f.toolLists = append(f.toolLists, toolList)
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

func agentWithLLM(m modelInterface, fb Feedback, cfg Config) *Agent {
	return &Agent{config: cfg, llm: m, fb: fb}
}

func skillCollection(t *testing.T) *skills.Collection {
	t.Helper()

	path := t.TempDir()
	content := "---\nname: git-release\ndescription: Draft release notes\n---\n\nDo the thing.\n"
	require.NoError(t, os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o600))

	collection := skills.NewCollection()
	require.NoError(t, collection.Add(path))

	return collection
}

func toolNames(toolBox *tools.ToolBox) []string {
	names := make([]string, 0)
	for _, tool := range toolBox.Tools() {
		names = append(names, tool.Name)
	}

	return names
}

func systemContent(t *testing.T, messages []llm.Message) string {
	t.Helper()

	system, ok := messages[0].(llm.SystemMessage)
	require.True(t, ok)

	return system.Content
}

type recordingFeedback struct {
	events []string
	tools  []string
}

func (f *recordingFeedback) ToolCalled(toolName string) {
	f.tools = append(f.tools, toolName)
	f.events = append(f.events, "ToolCalled")
}
func (f *recordingFeedback) ContextCompacted() { f.events = append(f.events, "ContextCompacted") }
func (f *recordingFeedback) ContextCompactionFailed() {
	f.events = append(f.events, "ContextCompactionFailed")
}
func (f *recordingFeedback) ModelInfoUnavailable() {
	f.events = append(f.events, "ModelInfoUnavailable")
}
func (f *recordingFeedback) SessionReset()   { f.events = append(f.events, "SessionReset") }
func (f *recordingFeedback) SessionStarted() { f.events = append(f.events, "SessionStarted") }
func (f *recordingFeedback) SessionClosed()  { f.events = append(f.events, "SessionClosed") }

func mustTestLLM(t testing.TB) *llm.LLM {
	t.Helper()

	client, err := llm.New(llm.Config{Provider: llm.ProviderOllama, Model: "test-model"})
	if err != nil {
		t.Fatalf("llm.New: unexpected error: %v", err)
	}

	return client
}

func mustNewTestAgent(t testing.TB, cfg Config, fb Feedback) *Agent {
	t.Helper()

	agt, err := New(cfg, mustTestLLM(t))
	if err != nil {
		t.Fatalf("New(%+v): unexpected error: %v", cfg, err)
	}

	agt.SetFeedback(fb)

	return agt
}

func TestNew(t *testing.T) {
	t.Run("returns an agent for a valid configuration", func(t *testing.T) {
		// when
		result, err := New(Config{}, mustTestLLM(t))
		// then
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("propagates ErrNoLLM when llm is nil", func(t *testing.T) {
		// when
		result, err := New(Config{}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrNoLLM)
	})

	t.Run("returns ErrInvalidThreshold when the percent is negative", func(t *testing.T) {
		// when
		result, err := New(Config{CompactionThresholdPercent: -1}, mustTestLLM(t))
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrInvalidThreshold)
	})

	t.Run("returns ErrInvalidThreshold when the percent exceeds one hundred", func(t *testing.T) {
		// when
		result, err := New(Config{CompactionThresholdPercent: 101}, mustTestLLM(t))
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrInvalidThreshold)
	})

	t.Run("succeeds for percents within range", func(t *testing.T) {
		for _, pct := range []int{0, 50, 100} {
			// when
			result, err := New(Config{CompactionThresholdPercent: pct}, mustTestLLM(t))
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
		agt := mustNewTestAgent(t, Config{}, fb)
		// when
		agt.StartSession(SessionConfig{Prompt: "be terse"})
		// then
		assert.Equal(t, []string{"SessionStarted"}, fb.events)
	})

	t.Run("offers the session's tools to the model", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		result := fake.toolLists[0]
		expected := []llm.Tool{{Name: "echo"}}
		assert.Equal(t, expected, result)
	})

	t.Run("offers no tools when the session has no ToolBox", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Empty(t, fake.toolLists[0])
	})

	t.Run("replaces the previous session's tools", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		first := tools.NewToolBox()
		require.NoError(t, first.Add(llm.Tool{Name: "first"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		second := tools.NewToolBox()
		require.NoError(t, second.Add(llm.Tool{Name: "second"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: first})
		// when
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: second})
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		result := fake.toolLists[0]
		expected := []llm.Tool{{Name: "second"}}
		assert.Equal(t, expected, result)
	})

	t.Run("adds the skill catalog to the system message", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "be terse", Skills: skillCollection(t)})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		result := systemContent(t, fake.calls[0])
		assert.True(t, strings.HasPrefix(result, "be terse\n\n"))
		assert.Contains(t, result, "<name>git-release</name>")
	})

	t.Run("leaves the system message alone when the collection is empty", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "be terse", Skills: skills.NewCollection()})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "be terse", systemContent(t, fake.calls[0]))
	})

	t.Run("registers the collection's tools in the session ToolBox", func(t *testing.T) {
		// given
		tb := tools.NewToolBox()
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		// when
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb, Skills: skillCollection(t)})
		// then
		result := toolNames(tb)
		expected := []string{"skill_load", "skill_load_file", "skill_execute_file"}
		assert.ElementsMatch(t, expected, result)
	})

	t.Run("offers the skill tools when the session has no ToolBox", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", Skills: skillCollection(t)})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		var names []string
		for _, tool := range fake.toolLists[0] {
			names = append(names, tool.Name)
		}
		assert.ElementsMatch(t, []string{"skill_load", "skill_load_file", "skill_execute_file"}, names)
	})

	t.Run("unregisters the previous session's skills", func(t *testing.T) {
		// given
		tb := tools.NewToolBox()
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb, Skills: skillCollection(t)})
		// when
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// then
		assert.Empty(t, toolNames(tb))
	})
}

func TestResetSession(t *testing.T) {
	t.Run("returns ErrNoSession before a session has started", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := mustNewTestAgent(t, Config{}, fb)
		// when
		err := agt.ResetSession()
		// then
		assert.ErrorIs(t, err, ErrNoSession)
		assert.Empty(t, fb.events)
	})

	t.Run("succeeds and fires SessionReset after a session has started", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := mustNewTestAgent(t, Config{}, fb)
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		err := agt.ResetSession()
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"SessionStarted", "SessionReset"}, fb.events)
	})
}

func TestClose(t *testing.T) {
	t.Run("unregisters the session's skills", func(t *testing.T) {
		// given
		tb := tools.NewToolBox()
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb, Skills: skillCollection(t)})
		// when
		agt.Close()
		// then
		assert.Empty(t, toolNames(tb))
	})

	t.Run("fires the SessionClosed event", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := mustNewTestAgent(t, Config{}, fb)
		agt.StartSession(SessionConfig{Prompt: "sys"})
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
		agt := mustNewTestAgent(t, Config{}, old)
		// when
		agt.SetFeedback(replacement)
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// then
		assert.Empty(t, old.events)
		assert.Equal(t, []string{"SessionStarted"}, replacement.events)
	})

	t.Run("keeps the existing sink when fb is nil", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := mustNewTestAgent(t, Config{}, fb)
		// when
		agt.SetFeedback(nil)
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// then
		assert.Equal(t, []string{"SessionStarted"}, fb.events)
	})
}

func TestProcess(t *testing.T) {
	t.Run("returns ErrNoSession when no session has started", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrNoSession)
	})

	t.Run("returns ErrNoSession after the session is closed", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		agt.Close()
		// when
		result, err := agt.Process(t.Context(), "hi")
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
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "hello", result.Content)
		assert.Equal(t, 15, result.Metadata.TotalTokens)
		assert.Equal(t, 0, result.Metadata.Iterations)
	})

	t.Run("surfaces the model's stop reason in the metadata", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "cut short", StopReason: "max_tokens"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "max_tokens", result.Metadata.StopReason)
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
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		result, err := agt.Process(t.Context(), "hi")
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
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{MaxIterations: 2})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrMaxIterations)
	})

	t.Run("propagates an error from the model", func(t *testing.T) {
		// given
		fake := &fakeLLM{chatErr: errors.New("boom")}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("compacts older turns once the threshold is crossed", func(t *testing.T) {
		// given: two small turns, then a third that trips 90% of the 1000-token
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
		agt := agentWithLLM(fake, fb, Config{CompactionThresholdPercent: 90})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when: three turns build enough history for compaction to have an older turn
		_, err1 := agt.Process(t.Context(), "u1")
		_, err2 := agt.Process(t.Context(), "u2")
		_, err3 := agt.Process(t.Context(), "u3")
		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		require.NoError(t, err3)
		assert.Contains(t, fb.events, "ContextCompacted")
		assert.Equal(t, 4, fake.chatCalls)
	})

	t.Run("compacts after a tool round when the threshold is crossed", func(t *testing.T) {
		// given: two completed turns, then a tool round whose final reply trips
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{Content: "a1", Stats: llm.Stats{TotalTokens: 100}},
				{Content: "a2", Stats: llm.Stats{TotalTokens: 100}},
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo"}}, Stats: llm.Stats{TotalTokens: 400}},
				{Content: "done", Stats: llm.Stats{TotalTokens: 950}},
				{Content: "SUMMARY"},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, fb, Config{CompactionThresholdPercent: 90})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		_, err := agt.Process(t.Context(), "u1")
		require.NoError(t, err)
		_, err = agt.Process(t.Context(), "u2")
		require.NoError(t, err)
		// when
		result, err := agt.Process(t.Context(), "u3")
		// then: the round returns its final reply and compaction runs after it
		require.NoError(t, err)
		assert.Equal(t, "done", result.Content)
		assert.Equal(t, []string{"echo"}, fb.tools)
		assert.Contains(t, fb.events, "ContextCompacted")
		assert.Equal(t, 5, fake.chatCalls)
	})

	t.Run("summarizes without repeating the system prompt", func(t *testing.T) {
		// given: same shape as the threshold test, watching the summarizer call
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
		agt := agentWithLLM(fake, fb, Config{CompactionThresholdPercent: 90})
		agt.StartSession(SessionConfig{Prompt: "the-session-prompt"})
		// when
		_, err1 := agt.Process(t.Context(), "u1")
		_, err2 := agt.Process(t.Context(), "u2")
		_, err3 := agt.Process(t.Context(), "u3")
		// then: the summarizer sees its own system prompt and a transcript
		require.NoError(t, err1)
		require.NoError(t, err2)
		require.NoError(t, err3)
		require.Contains(t, fb.events, "ContextCompacted")
		require.Len(t, fake.calls, 4)
		summaryCall := fake.calls[3]
		require.Len(t, summaryCall, 2)
		assert.Equal(t, llm.SystemMessage{Content: summarySystemPrompt}, summaryCall[0])
		transcript := summaryCall[1].(llm.UserMessage)
		assert.NotContains(t, transcript.Content, "the-session-prompt")
	})

	t.Run("reports every turn the model info stays unavailable", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "one"}, {Content: "two"}},
			infoErr: errors.New("no info"),
		}
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when: two turns both fail to fetch the model info
		_, err1 := agt.Process(t.Context(), "u1")
		_, err2 := agt.Process(t.Context(), "u2")
		// then: the fetch is retried every turn, so the event fires each time
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, []string{"SessionStarted", "ModelInfoUnavailable", "ModelInfoUnavailable"}, fb.events)
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
		require.NoError(t, tb.Add(llm.Tool{Name: "boom"}, func(context.Context, map[string]any) (string, error) {
			return "", errors.New("kaboom")
		}))
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "recovered", result.Content)
		assert.Equal(t, 1, result.Metadata.ToolCalls)
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
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi", Stats: llm.Stats{TotalTokens: 9999}}},
			infoErr: errors.New("no info"),
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{CompactionThresholdPercent: 1})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result, err := agt.Process(t.Context(), "hi")
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
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
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
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		_, err := agt.Process(t.Context(), "hi")
		require.NoError(t, err)
		require.Equal(t, 1000, agt.modelInfo.ContextSize)
		// when
		err = agt.ChangeModel("m2")
		// then
		require.NoError(t, err)
		assert.Equal(t, "m2", fake.current)
		assert.Nil(t, agt.modelInfo)
		assert.Zero(t, agt.compactThreshold)
	})

	t.Run("propagates the client error and keeps the current model", func(t *testing.T) {
		// given
		fake := &fakeLLM{changeErr: errors.New("boom"), current: "old"}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		// when
		err := agt.ChangeModel("m2")
		// then
		assert.ErrorContains(t, err, "boom")
		assert.Equal(t, "old", fake.current)
	})
}

func TestModelInfo(t *testing.T) {
	t.Run("reports the active model's provider, name, context window, and effort", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			info:   &llm.ModelInfo{Provider: llm.ProviderAnthropic, Name: "m1", ContextSize: 2000},
			effort: llm.EffortMedium,
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		// when
		result := agt.ModelInfo(t.Context())
		// then
		require.NotNil(t, result)
		assert.Equal(t, llm.ProviderAnthropic, result.Provider)
		assert.Equal(t, "m1", result.ModelName)
		assert.Equal(t, 2000, result.ModelContextSize)
		assert.Equal(t, llm.EffortMedium, result.Effort)
	})

	t.Run("returns nil when the model info cannot be fetched", func(t *testing.T) {
		// given: called before any turn, with a client that cannot report info
		fake := &fakeLLM{infoErr: errors.New("no info")}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		// when
		result := agt.ModelInfo(t.Context())
		// then
		assert.Nil(t, result)
	})
}

func TestChangeEffort(t *testing.T) {
	t.Run("sets the effort on the underlying client", func(t *testing.T) {
		// given
		fake := &fakeLLM{}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		// when
		agt.ChangeEffort(llm.EffortMax)
		// then
		assert.Equal(t, llm.EffortMax, fake.effort)
	})
}

func TestCompactContext(t *testing.T) {
	t.Run("keeps the skill catalog in the system message", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{Content: "a1", Stats: llm.Stats{TotalTokens: 10}},
				{Content: "a2", Stats: llm.Stats{TotalTokens: 20}},
				{Content: "SUMMARY"},
				{Content: "a3", Stats: llm.Stats{TotalTokens: 30}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "be terse", Skills: skillCollection(t)})
		_, err1 := agt.Process(t.Context(), "u1")
		_, err2 := agt.Process(t.Context(), "u2")
		require.NoError(t, err1)
		require.NoError(t, err2)
		// when
		agt.CompactContext(t.Context())
		_, err3 := agt.Process(t.Context(), "u3")
		// then
		require.NoError(t, err3)
		result := systemContent(t, fake.calls[len(fake.calls)-1])
		assert.Contains(t, result, "<name>git-release</name>")
	})

	t.Run("is a no-op when there is no older turn to summarize", func(t *testing.T) {
		// given: a single completed turn — nothing older than the kept window
		fb := &recordingFeedback{}
		fake := &fakeLLM{}
		agt := agentWithLLM(fake, fb, Config{})
		agt.messages = []llm.Message{
			llm.SystemMessage{Content: "sys"},
			llm.UserMessage{Content: "u1"},
			llm.AssistantMessage{Content: "a1"},
		}
		before := len(agt.messages)
		// when
		agt.CompactContext(t.Context())
		// then
		assert.Len(t, agt.messages, before)
		assert.Zero(t, fake.chatCalls)
		assert.NotContains(t, fb.events, "ContextCompacted")
	})

	t.Run("leaves the conversation unchanged when summarization fails", func(t *testing.T) {
		// given: enough turns that an older one exists, but the summarizing Chat errors
		fb := &recordingFeedback{}
		fake := &fakeLLM{chatErr: errors.New("summary boom")}
		agt := agentWithLLM(fake, fb, Config{})
		agt.messages = []llm.Message{
			llm.SystemMessage{Content: "sys"},
			llm.UserMessage{Content: "u1"}, llm.AssistantMessage{Content: "a1"},
			llm.UserMessage{Content: "u2"}, llm.AssistantMessage{Content: "a2"},
			llm.UserMessage{Content: "u3"}, llm.AssistantMessage{Content: "a3"},
		}
		snapshot := append([]llm.Message(nil), agt.messages...)
		// when
		agt.CompactContext(t.Context())
		// then
		assert.Equal(t, snapshot, agt.messages)
		assert.NotContains(t, fb.events, "ContextCompacted")
		assert.Contains(t, fb.events, "ContextCompactionFailed")
	})
}
