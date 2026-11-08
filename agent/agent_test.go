package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/skills"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLLM struct {
	replies   []*llm.AssistantMessage
	chatErr   error
	info      *llm.ModelInfo
	chatCalls int
	calls     [][]llm.Message
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

func (f *fakeLLM) ModelInfo() *llm.ModelInfo {
	if f.info == nil {
		return &llm.ModelInfo{}
	}
	return f.info
}

func agentWithLLM(m Model, fb Feedback, cfg Config) *Agent {
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

type toolReturn struct {
	tool    string
	result  string
	err     error
	elapsed time.Duration
}

type recordingFeedback struct {
	events     []string
	tools      []string
	toolArgs   []map[string]any
	returns    []toolReturn
	stats      []int
	cached     []int
	interim    []string
	resumedIDs []string
}

func (f *recordingFeedback) ToolCalled(toolName string, args map[string]any) {
	f.tools = append(f.tools, toolName)
	f.toolArgs = append(f.toolArgs, args)
	f.events = append(f.events, "ToolCalled")
}

func (f *recordingFeedback) ToolReturned(toolName string, result string, err error, elapsed time.Duration) {
	f.returns = append(f.returns, toolReturn{tool: toolName, result: result, err: err, elapsed: elapsed})
	f.events = append(f.events, "ToolReturned")
}
func (f *recordingFeedback) InterimTextReceived(content string) {
	f.interim = append(f.interim, content)
	f.events = append(f.events, "InterimTextReceived")
}
func (f *recordingFeedback) TokensUsed(totalTokens, cacheReadTokens int) {
	f.stats = append(f.stats, totalTokens)
	f.cached = append(f.cached, cacheReadTokens)
	f.events = append(f.events, "TokensUsed")
}
func (f *recordingFeedback) ContextCompacted() { f.events = append(f.events, "ContextCompacted") }
func (f *recordingFeedback) ContextCompactionFailed() {
	f.events = append(f.events, "ContextCompactionFailed")
}
func (f *recordingFeedback) SessionReset()   { f.events = append(f.events, "SessionReset") }
func (f *recordingFeedback) SessionStarted() { f.events = append(f.events, "SessionStarted") }
func (f *recordingFeedback) SessionClosed()  { f.events = append(f.events, "SessionClosed") }
func (f *recordingFeedback) SessionResumed(sessionID string) {
	f.resumedIDs = append(f.resumedIDs, sessionID)
	f.events = append(f.events, "SessionResumed")
}

func mustNewTestAgent(t testing.TB, cfg Config, fb Feedback) *Agent {
	t.Helper()

	agt, err := New(cfg, &fakeLLM{})
	if err != nil {
		t.Fatalf("New(%+v): unexpected error: %v", cfg, err)
	}

	agt.SetFeedback(fb)

	return agt
}

func TestNew(t *testing.T) {
	t.Run("propagates ErrNoLLM when llm is nil", func(t *testing.T) {
		// when
		result, err := New(Config{}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrNoLLM)
	})

	t.Run("returns ErrInvalidThreshold when the percent is negative", func(t *testing.T) {
		// when
		result, err := New(Config{CompactionThresholdPercent: -1}, &fakeLLM{})
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrInvalidThreshold)
	})

	t.Run("returns ErrInvalidThreshold when the percent exceeds one hundred", func(t *testing.T) {
		// when
		result, err := New(Config{CompactionThresholdPercent: 101}, &fakeLLM{})
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrInvalidThreshold)
	})

	t.Run("succeeds for percents within range", func(t *testing.T) {
		for _, pct := range []int{0, 50, 100} {
			// when
			result, err := New(Config{CompactionThresholdPercent: pct}, &fakeLLM{})
			// then
			require.NoError(t, err, "pct=%d", pct)
			assert.NotNil(t, result, "pct=%d", pct)
		}
	})

	t.Run("runs on any Model, not only an llm.LLM", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hello"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		agt, err := New(Config{}, fake)
		require.NoError(t, err)
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "hello", result.Content)
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

	t.Run("fires the SessionResumed event with the session id when it restores turns", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := mustNewTestAgent(t, Config{}, fb)
		restored := []llm.Message{
			llm.UserMessage{Content: "hi"},
			llm.AssistantMessage{Content: "hello"},
		}
		// when
		agt.StartSession(SessionConfig{Prompt: "sys", Messages: restored, ID: "saved"})
		// then
		assert.Equal(t, []string{"SessionResumed"}, fb.events)
		assert.Equal(t, []string{"saved"}, fb.resumedIDs)
	})

	t.Run("fires the SessionStarted event when the restored messages are all system messages", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		agt := mustNewTestAgent(t, Config{}, fb)
		restored := []llm.Message{
			llm.SystemMessage{Content: "old prompt"},
		}
		// when
		agt.StartSession(SessionConfig{Prompt: "sys", Messages: restored})
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

	t.Run("sends the restored messages after the system message", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "done"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		call := llm.ToolCall{ID: "c1", Name: "echo", Arguments: map[string]any{"text": "hi"}}
		restored := []llm.Message{
			llm.UserMessage{Content: "hi"},
			llm.AssistantMessage{ToolCalls: []llm.ToolCall{call}},
			llm.ToolMessage{ToolCallID: "c1", ToolName: "echo", Content: "hi"},
			llm.AssistantMessage{Content: "said hi"},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", Messages: restored})
		// when
		_, err := agt.Process(t.Context(), "again")
		// then
		require.NoError(t, err)
		result := fake.calls[0]
		expected := append([]llm.Message{llm.SystemMessage{Content: "sys"}}, restored...)
		expected = append(expected, llm.UserMessage{Content: "again"})
		assert.Equal(t, expected, result)
	})

	t.Run("replaces a restored system message with the new prompt", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "done"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		restored := []llm.Message{
			llm.SystemMessage{Content: "old prompt"},
			llm.UserMessage{Content: "hi"},
			llm.AssistantMessage{Content: "hello"},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "new prompt", Messages: restored})
		// when
		_, err := agt.Process(t.Context(), "")
		// then
		require.NoError(t, err)
		result := fake.calls[0]
		expected := []llm.Message{
			llm.SystemMessage{Content: "new prompt"},
			llm.UserMessage{Content: "hi"},
			llm.AssistantMessage{Content: "hello"},
		}
		assert.Equal(t, expected, result)
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

	t.Run("discards the restored messages", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "done"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		restored := []llm.Message{
			llm.UserMessage{Content: "hi"},
			llm.AssistantMessage{Content: "hello"},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", Messages: restored})
		// when
		err := agt.ResetSession()
		// then
		require.NoError(t, err)
		_, err = agt.Process(t.Context(), "again")
		require.NoError(t, err)
		result := fake.calls[0]
		expected := []llm.Message{
			llm.SystemMessage{Content: "sys"},
			llm.UserMessage{Content: "again"},
		}
		assert.Equal(t, expected, result)
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

func TestMessages(t *testing.T) {
	t.Run("returns nil before a session has started", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		// when
		result := agt.Messages()
		// then
		assert.Nil(t, result)
	})

	t.Run("returns the whole conversation after a round with a tool call", func(t *testing.T) {
		// given
		call := llm.ToolCall{ID: "c1", Name: "echo"}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{call}},
				{Content: "done"},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		_, err := agt.Process(t.Context(), "hi")
		require.NoError(t, err)
		// when
		result := agt.Messages()
		// then
		expected := []llm.Message{
			llm.SystemMessage{Content: "sys"},
			llm.UserMessage{Content: "hi"},
			llm.AssistantMessage{ToolCalls: []llm.ToolCall{call}},
			llm.ToolMessage{ToolCallID: "c1", ToolName: "echo", Content: "ok"},
			llm.AssistantMessage{Content: "done"},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("returns a copy the caller can change freely", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "done"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result := agt.Messages()
		result[0] = llm.SystemMessage{Content: "changed"}
		// then
		_, err := agt.Process(t.Context(), "hi")
		require.NoError(t, err)
		assert.Equal(t, "sys", systemContent(t, fake.calls[0]))
	})

	t.Run("returns nil after Close", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		agt.Close()
		// when
		result := agt.Messages()
		// then
		assert.Nil(t, result)
	})

	t.Run("resumes in another agent under its own prompt", func(t *testing.T) {
		// given
		first := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hello"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		source := agentWithLLM(first, &recordingFeedback{}, Config{})
		source.StartSession(SessionConfig{Prompt: "old prompt"})
		_, err := source.Process(t.Context(), "hi")
		require.NoError(t, err)
		second := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "welcome back"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		target := agentWithLLM(second, &recordingFeedback{}, Config{})
		// when
		target.StartSession(SessionConfig{Prompt: "new prompt", Messages: source.Messages()})
		// then
		_, err = target.Process(t.Context(), "again")
		require.NoError(t, err)
		result := second.calls[0]
		expected := []llm.Message{
			llm.SystemMessage{Content: "new prompt"},
			llm.UserMessage{Content: "hi"},
			llm.AssistantMessage{Content: "hello"},
			llm.UserMessage{Content: "again"},
		}
		assert.Equal(t, expected, result)
	})
}

func TestSessionID(t *testing.T) {
	t.Run("returns empty before a session has started", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		// when
		result := agt.SessionID()
		// then
		assert.Empty(t, result)
	})

	t.Run("returns a new token after StartSession", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result := agt.SessionID()
		// then
		assert.True(t, token.Valid(result))
	})

	t.Run("returns SessionConfig.ID when it is set", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession(SessionConfig{Prompt: "sys", ID: "my-session"})
		// when
		result := agt.SessionID()
		// then
		assert.Equal(t, "my-session", result)
	})

	t.Run("returns a different id for each new session", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		previous := agt.SessionID()
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result := agt.SessionID()
		// then
		assert.True(t, token.Valid(result))
		assert.NotEqual(t, previous, result)
	})

	t.Run("returns a new token after ResetSession", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession(SessionConfig{Prompt: "sys", ID: "my-session"})
		require.NoError(t, agt.ResetSession())
		// when
		result := agt.SessionID()
		// then
		assert.True(t, token.Valid(result))
		assert.NotEqual(t, "my-session", result)
	})

	t.Run("stays empty when ResetSession has no session", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		require.ErrorIs(t, agt.ResetSession(), ErrNoSession)
		// when
		result := agt.SessionID()
		// then
		assert.Empty(t, result)
	})

	t.Run("returns empty after Close", func(t *testing.T) {
		// given
		agt := mustNewTestAgent(t, Config{}, &recordingFeedback{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		agt.Close()
		// when
		result := agt.SessionID()
		// then
		assert.Empty(t, result)
	})
}

func TestLLM(t *testing.T) {
	t.Run("returns the llm.LLM passed to New", func(t *testing.T) {
		// given
		expected := &llm.LLM{}
		agt, err := New(Config{}, expected)
		require.NoError(t, err)
		// when
		result := agt.LLM()
		// then
		assert.Same(t, expected, result)
	})

	t.Run("returns nil for another model", func(t *testing.T) {
		// given
		agt, err := New(Config{}, &fakeLLM{})
		require.NoError(t, err)
		// when
		result := agt.LLM()
		// then
		assert.Nil(t, result)
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

	t.Run("copies the cache-read tokens into the metadata", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{Content: "hello", Stats: llm.Stats{PromptTokens: 100, CacheReadTokens: 80, TotalTokens: 110}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		expected := 80
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, result.Metadata.CacheReadTokens)
	})

	t.Run("runs requested tools and feeds the results back", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: map[string]any{"text": "hi"}}}},
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
		assert.Equal(t, []map[string]any{{"text": "hi"}}, fb.toolArgs)
	})

	t.Run("fires TokensUsed for each intermediate response, but not the final one", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo"}}, Stats: llm.Stats{PromptTokens: 10, OutputTokens: 5, TotalTokens: 15}},
				{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "echo"}}, Stats: llm.Stats{PromptTokens: 20, OutputTokens: 10, TotalTokens: 30}},
				{Content: "done", Stats: llm.Stats{TotalTokens: 20}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, []int{15, 30}, fb.stats)
		assert.Equal(t, []string{"SessionStarted", "TokensUsed", "ToolCalled", "ToolReturned", "TokensUsed", "ToolCalled", "ToolReturned"}, fb.events)
	})

	t.Run("passes each intermediate response's cache-read tokens to TokensUsed", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo"}}, Stats: llm.Stats{PromptTokens: 10, CacheReadTokens: 4, TotalTokens: 15}},
				{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "echo"}}, Stats: llm.Stats{PromptTokens: 20, CacheReadTokens: 8, TotalTokens: 30}},
				{Content: "done", Stats: llm.Stats{TotalTokens: 20}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		expected := []int{4, 8}
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, fb.cached)
	})

	t.Run("fires InterimTextReceived before TokensUsed for text sent with tool calls", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{Content: "Let me check", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo"}}},
				{Content: "done"},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Let me check"}, fb.interim)
		assert.Equal(t, []string{"SessionStarted", "InterimTextReceived", "TokensUsed", "ToolCalled", "ToolReturned"}, fb.events)
	})

	t.Run("does not fire InterimTextReceived when tool calls come without text", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo"}}},
				{Content: "done"},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Empty(t, fb.interim)
	})

	t.Run("does not fire InterimTextReceived for the final answer", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "done"}},
			info:    &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "done", result.Content)
		assert.Empty(t, fb.interim)
	})

	t.Run("fires ToolReturned with the tool's result after the call", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: map[string]any{"text": "hi"}}}},
				{Content: "done", Stats: llm.Stats{TotalTokens: 20}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) {
			return "ok", nil
		}))
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		require.Len(t, fb.returns, 1)
		assert.Equal(t, "echo", fb.returns[0].tool)
		assert.Equal(t, "ok", fb.returns[0].result)
		assert.NoError(t, fb.returns[0].err)
		assert.Positive(t, fb.returns[0].elapsed)
	})

	t.Run("fires ToolReturned with the error and no result when the tool fails", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "boom"}}},
				{Content: "done", Stats: llm.Stats{TotalTokens: 20}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "boom"}, func(context.Context, map[string]any) (string, error) {
			return "", errors.New("nope")
		}))
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		require.Len(t, fb.returns, 1)
		assert.Equal(t, "boom", fb.returns[0].tool)
		assert.Empty(t, fb.returns[0].result)
		assert.ErrorContains(t, fb.returns[0].err, "nope")
	})

	t.Run("pairs every ToolCalled with a ToolReturned, in order", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{ToolCalls: []llm.ToolCall{
					{ID: "c1", Name: "echo"},
					{ID: "c2", Name: "echo"},
				}},
				{Content: "done", Stats: llm.Stats{TotalTokens: 20}},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		tb := tools.NewToolBox()
		require.NoError(t, tb.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) {
			return "ok", nil
		}))
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", ToolBox: tb})
		// when
		_, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"SessionStarted", "TokensUsed", "ToolCalled", "ToolReturned", "ToolCalled", "ToolReturned"}, fb.events)
	})

	t.Run("returns ErrMaxIterations when the iteration cap is reached", func(t *testing.T) {
		// given
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

	t.Run("compacts after a tool round when the threshold is crossed", func(t *testing.T) {
		// given
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
		// then
		require.NoError(t, err)
		assert.Equal(t, "done", result.Content)
		assert.Equal(t, []string{"echo"}, fb.tools)
		assert.Contains(t, fb.events, "ContextCompacted")
		assert.Equal(t, 5, fake.chatCalls)
	})

	t.Run("summarizes without repeating the system prompt", func(t *testing.T) {
		// given
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
		// then
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

	t.Run("feeds a failing tool's error back to the model and continues", func(t *testing.T) {
		// given
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
		sent := fake.calls[1]
		toolMsg, ok := sent[len(sent)-1].(llm.ToolMessage)
		require.True(t, ok)
		assert.Equal(t, "c1", toolMsg.ToolCallID)
		assert.Contains(t, toolMsg.Content, "kaboom")
	})

	t.Run("compacts with the context window the model reports after a model change", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{Content: "a1", Stats: llm.Stats{TotalTokens: 100}},
				{Content: "a2", Stats: llm.Stats{TotalTokens: 100}},
				{Content: "a3", Stats: llm.Stats{TotalTokens: 950}},
				{Content: "SUMMARY"},
			},
			info: &llm.ModelInfo{ContextSize: 100000},
		}
		agt := agentWithLLM(fake, fb, Config{CompactionThresholdPercent: 90})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		_, err1 := agt.Process(t.Context(), "u1")
		_, err2 := agt.Process(t.Context(), "u2")
		require.NoError(t, err1)
		require.NoError(t, err2)
		fake.info = &llm.ModelInfo{ContextSize: 1000}
		// when
		_, err := agt.Process(t.Context(), "u3")
		// then
		require.NoError(t, err)
		assert.Contains(t, fb.events, "ContextCompacted")
	})

	t.Run("skips compaction when the context size is unknown", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{{Content: "hi", Stats: llm.Stats{TotalTokens: 9999}}},
			info:    &llm.ModelInfo{ContextSize: 0},
		}
		fb := &recordingFeedback{}
		agt := agentWithLLM(fake, fb, Config{CompactionThresholdPercent: 1})
		agt.StartSession(SessionConfig{Prompt: "sys"})
		// when
		result, err := agt.Process(t.Context(), "hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "hi", result.Content)
		assert.NotContains(t, fb.events, "ContextCompacted")
		assert.Equal(t, 1, fake.chatCalls)
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

	t.Run("summarizes the turns restored as pointers", func(t *testing.T) {
		// given
		fake := &fakeLLM{
			replies: []*llm.AssistantMessage{
				{Content: "a2", Stats: llm.Stats{TotalTokens: 10}},
				{Content: "a3", Stats: llm.Stats{TotalTokens: 10}},
				{Content: "SUMMARY"},
			},
			info: &llm.ModelInfo{ContextSize: 1000},
		}
		agt := agentWithLLM(fake, &recordingFeedback{}, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", Messages: []llm.Message{
			&llm.UserMessage{Content: "u1"},
			&llm.AssistantMessage{Content: "a1"},
		}})
		_, err2 := agt.Process(t.Context(), "u2")
		_, err3 := agt.Process(t.Context(), "u3")
		require.NoError(t, err2)
		require.NoError(t, err3)
		// when
		agt.CompactContext(t.Context())
		// then
		summaryRequest, ok := fake.calls[len(fake.calls)-1][1].(llm.UserMessage)
		require.True(t, ok)
		assert.Contains(t, summaryRequest.Content, "User: u1")
		assert.Contains(t, summaryRequest.Content, "Assistant: a1")
	})

	t.Run("is a no-op when there is no older turn to summarize", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{}
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", Messages: []llm.Message{
			llm.UserMessage{Content: "u1"},
			llm.AssistantMessage{Content: "a1"},
		}})
		before := agt.Messages()
		// when
		agt.CompactContext(t.Context())
		// then
		assert.Equal(t, before, agt.Messages())
		assert.Zero(t, fake.chatCalls)
		assert.NotContains(t, fb.events, "ContextCompacted")
	})

	t.Run("leaves the conversation unchanged when summarization fails", func(t *testing.T) {
		// given
		fb := &recordingFeedback{}
		fake := &fakeLLM{chatErr: errors.New("summary boom")}
		agt := agentWithLLM(fake, fb, Config{})
		agt.StartSession(SessionConfig{Prompt: "sys", Messages: []llm.Message{
			llm.UserMessage{Content: "u1"}, llm.AssistantMessage{Content: "a1"},
			llm.UserMessage{Content: "u2"}, llm.AssistantMessage{Content: "a2"},
			llm.UserMessage{Content: "u3"}, llm.AssistantMessage{Content: "a3"},
		}})
		before := agt.Messages()
		// when
		agt.CompactContext(t.Context())
		// then
		assert.Equal(t, before, agt.Messages())
		assert.NotContains(t, fb.events, "ContextCompacted")
		assert.Contains(t, fb.events, "ContextCompactionFailed")
	})
}
