package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/jjmrocha/ai-toolkit/agent"
	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/skills"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/fn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	answerReply   = `{"message":{"content":"the answer"},"done":true,"prompt_eval_count":20,"eval_count":10}`
	toolCallReply = `{"message":{"content":"","tool_calls":[{"function":{"name":"echo","arguments":{"text":"hi"}}}]},"done":true,"prompt_eval_count":10,"eval_count":5}`
	errorReply    = `{"error":"boom"}`
	modelInfo     = `{"model_info":{"general.architecture":"test","test.context_length":8192}}`
	notesSkill    = "---\nname: notes\ndescription: Takes notes\n---\nWrite it down.\n"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatTool struct {
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

type chatRequest struct {
	Messages []chatMessage `json:"messages"`
	Tools    []chatTool    `json:"tools"`
}

func (r chatRequest) toolNames() []string {
	return fn.Map(r.Tools, func(tool chatTool) string { return tool.Function.Name })
}

type modelServer struct {
	mu       sync.Mutex
	replies  []string
	requests []chatRequest
}

func (s *modelServer) reply(request chatRequest) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = append(s.requests, request)

	reply := s.replies[0]
	if len(s.replies) > 1 {
		s.replies = s.replies[1:]
	}

	return reply
}

func (s *modelServer) chatRequests() []chatRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.requests)
}

type recordingFeedback struct {
	mu         sync.Mutex
	events     []string
	finishErrs []error
	cached     []int
}

func (f *recordingFeedback) record(event string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func (f *recordingFeedback) SubagentStarted(agentName string) {
	f.record("started " + agentName)
}

func (f *recordingFeedback) SubagentFinished(agentName string, err error) {
	f.record("finished " + agentName)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finishErrs = append(f.finishErrs, err)
}

func (f *recordingFeedback) ToolCalled(agentName string, toolName string) {
	f.record("tool " + agentName + " " + toolName)
}

func (f *recordingFeedback) TokensUsed(agentName string, totalTokens, cacheReadTokens int) {
	f.record(fmt.Sprintf("tokens %s %d", agentName, totalTokens))
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cached = append(f.cached, cacheReadTokens)
}

func (f *recordingFeedback) recordedCached() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.cached)
}

func (f *recordingFeedback) recorded() ([]string, []error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.events), slices.Clone(f.finishErrs)
}

func newModelServer(t *testing.T, replies ...string) (*llm.LLM, *modelServer) {
	t.Helper()

	recorder := &modelServer{replies: replies}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path != "/api/chat" {
			_, _ = io.WriteString(w, modelInfo)
			return
		}

		var request chatRequest

		_ = json.NewDecoder(r.Body).Decode(&request)

		_, _ = io.WriteString(w, recorder.reply(request))
	}))
	t.Cleanup(server.Close)

	model, err := llm.New(llm.Config{Provider: llm.ProviderOllama, Model: "test", BaseURL: server.URL})
	require.NoError(t, err)

	return model, recorder
}

func newChild(t *testing.T, replies ...string) (*agent.Agent, *modelServer) {
	t.Helper()

	model, server := newModelServer(t, replies...)

	child, err := agent.New(agent.Config{}, model)
	require.NoError(t, err)

	return child, server
}

func newEchoToolBox(t *testing.T, calls *[]map[string]any) *tools.ToolBox {
	t.Helper()

	echoTool := llm.Tool{
		Name:        "echo",
		Description: "Echoes text",
		Schema:      tools.NewObjectBuilder().String("text", "the text", true).Build(),
	}

	toolBox := tools.NewToolBox()

	err := toolBox.Add(echoTool, func(_ context.Context, args map[string]any) (string, error) {
		*calls = append(*calls, args)
		return "echoed", nil
	})
	require.NoError(t, err)

	return toolBox
}

func newNotesSkills(t *testing.T) *skills.Collection {
	t.Helper()

	path := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(notesSkill), 0o600))

	collection := skills.NewCollection()
	require.NoError(t, collection.Add(path))

	return collection
}

func TestNewManager(t *testing.T) {
	t.Run("returns a manager with no agents", func(t *testing.T) {
		// when
		result := NewManager()
		// then
		require.NotNil(t, result)
		assert.Empty(t, result.Agents())
	})
}

func TestManagerRegister(t *testing.T) {
	t.Run("rejects an invalid config and registers nothing", func(t *testing.T) {
		child, _ := newChild(t, answerReply)

		tests := map[string]struct {
			cfg      Config
			agent    *agent.Agent
			expected error
		}{
			"empty name": {
				cfg:      Config{Description: "Finds code"},
				agent:    child,
				expected: ErrNoName,
			},
			"no agent": {
				cfg:      Config{Name: "explore"},
				expected: ErrNoAgent,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				// given
				manager := NewManager()
				// when
				err := manager.Register(tt.cfg, tt.agent, nil, nil)
				// then
				assert.ErrorIs(t, err, tt.expected)
				assert.Empty(t, manager.Agents())
			})
		}
	})

	t.Run("accepts a config with only a name and an agent", func(t *testing.T) {
		// given
		child, _ := newChild(t, answerReply)
		manager := NewManager()
		// when
		err := manager.Register(Config{Name: "explore"}, child, nil, nil)
		// then
		require.NoError(t, err)
		expected := []Info{{Name: "explore"}}
		assert.Equal(t, expected, manager.Agents())
	})

	t.Run("replaces the agent registered under the same name", func(t *testing.T) {
		// given
		child, _ := newChild(t, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore", Description: "old"}, child, nil, nil))
		// when
		err := manager.Register(Config{Name: "explore", Description: "new"}, child, nil, nil)
		// then
		require.NoError(t, err)
		expected := []Info{{Name: "explore", Description: "new"}}
		assert.Equal(t, expected, manager.Agents())
	})
}

func TestManagerAgents(t *testing.T) {
	t.Run("returns every agent sorted by name", func(t *testing.T) {
		// given
		child, _ := newChild(t, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "review", Description: "Reviews code"}, child, nil, nil))
		require.NoError(t, manager.Register(Config{Name: "explore", Description: "Finds code"}, child, nil, nil))
		// when
		result := manager.Agents()
		// then
		expected := []Info{
			{Name: "explore", Description: "Finds code"},
			{Name: "review", Description: "Reviews code"},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("returns nothing when no agent is registered", func(t *testing.T) {
		// given
		manager := NewManager()
		// when
		result := manager.Agents()
		// then
		assert.Empty(t, result)
	})
}

func TestManagerRun(t *testing.T) {
	t.Run("returns the child's final answer", func(t *testing.T) {
		// given
		child, _ := newChild(t, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
		// when
		result, err := manager.Run(t.Context(), "explore", "find the parser")
		// then
		require.NoError(t, err)
		assert.Equal(t, "the answer", result)
	})

	t.Run("sends the agent's prompt as the system message and the task as the user message", func(t *testing.T) {
		// given
		child, server := newChild(t, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore", Prompt: "You explore code."}, child, nil, nil))
		// when
		_, err := manager.Run(t.Context(), "explore", "find the parser")
		// then
		require.NoError(t, err)
		requests := server.chatRequests()
		require.Len(t, requests, 1)
		expected := []chatMessage{
			{Role: "system", Content: "You explore code."},
			{Role: "user", Content: "find the parser"},
		}
		assert.Equal(t, expected, requests[0].Messages)
	})

	t.Run("starts every run with a fresh conversation", func(t *testing.T) {
		// given
		child, server := newChild(t, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore", Prompt: "You explore code."}, child, nil, nil))
		_, err := manager.Run(t.Context(), "explore", "first task")
		require.NoError(t, err)
		// when
		_, err = manager.Run(t.Context(), "explore", "second task")
		// then
		require.NoError(t, err)
		requests := server.chatRequests()
		require.Len(t, requests, 2)
		expected := []chatMessage{
			{Role: "system", Content: "You explore code."},
			{Role: "user", Content: "second task"},
		}
		assert.Equal(t, expected, requests[1].Messages)
	})

	t.Run("runs the child's tool calls against the agent's ToolBox", func(t *testing.T) {
		// given
		var calls []map[string]any

		child, server := newChild(t, toolCallReply, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, newEchoToolBox(t, &calls), nil))
		// when
		result, err := manager.Run(t.Context(), "explore", "say hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, "the answer", result)
		expectedCalls := []map[string]any{{"text": "hi"}}
		assert.Equal(t, expectedCalls, calls)
		requests := server.chatRequests()
		require.Len(t, requests, 2)
		assert.Equal(t, []string{"echo"}, requests[0].toolNames())
		expected := chatMessage{Role: "tool", Content: "echoed"}
		assert.Contains(t, requests[1].Messages, expected)
	})

	t.Run("offers the child no tools when the agent has no ToolBox", func(t *testing.T) {
		// given
		child, server := newChild(t, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
		// when
		_, err := manager.Run(t.Context(), "explore", "find the parser")
		// then
		require.NoError(t, err)
		requests := server.chatRequests()
		require.Len(t, requests, 1)
		assert.Empty(t, requests[0].Tools)
	})

	t.Run("offers the agent's skills during the run and removes their tools afterwards", func(t *testing.T) {
		// given
		child, server := newChild(t, answerReply)
		toolBox := tools.NewToolBox()
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, toolBox, newNotesSkills(t)))
		// when
		_, err := manager.Run(t.Context(), "explore", "take a note")
		// then
		require.NoError(t, err)
		requests := server.chatRequests()
		require.Len(t, requests, 1)
		assert.Contains(t, requests[0].toolNames(), "skill_load")
		assert.Contains(t, requests[0].Messages[0].Content, "Takes notes")
		assert.Empty(t, toolBox.Tools())
	})

	t.Run("removes the skill tools when the child fails", func(t *testing.T) {
		// given
		child, _ := newChild(t, errorReply)
		toolBox := tools.NewToolBox()
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, toolBox, newNotesSkills(t)))
		// when
		_, err := manager.Run(t.Context(), "explore", "take a note")
		// then
		require.Error(t, err)
		assert.Empty(t, toolBox.Tools())
	})

	t.Run("returns an empty answer when the child ends without text", func(t *testing.T) {
		// given
		child, _ := newChild(t, `{"message":{"content":""},"done":true}`)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
		// when
		result, err := manager.Run(t.Context(), "explore", "find the parser")
		// then
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("rejects an empty prompt without calling the model", func(t *testing.T) {
		// given
		child, server := newChild(t, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
		// when
		result, err := manager.Run(t.Context(), "explore", "")
		// then
		assert.ErrorIs(t, err, ErrNoPrompt)
		assert.Empty(t, result)
		assert.Empty(t, server.chatRequests())
	})

	t.Run("names the missing agent when the agent is unknown", func(t *testing.T) {
		// given
		child, server := newChild(t, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "review"}, child, nil, nil))
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
		// when
		result, err := manager.Run(t.Context(), "plan", "find the parser")
		// then
		require.ErrorIs(t, err, ErrAgentNotFound)
		expected := "subagent: agent not found: plan"
		assert.EqualError(t, err, expected)
		assert.Empty(t, result)
		assert.Empty(t, server.chatRequests())
	})

	t.Run("returns the model's error", func(t *testing.T) {
		// given
		child, _ := newChild(t, errorReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
		// when
		result, err := manager.Run(t.Context(), "explore", "find the parser")
		// then
		assert.ErrorContains(t, err, "boom")
		assert.Empty(t, result)
	})

	t.Run("returns ErrMaxIterations when the child reaches the agent's iteration cap", func(t *testing.T) {
		// given
		var calls []map[string]any

		model, _ := newModelServer(t, toolCallReply)
		child, err := agent.New(agent.Config{MaxIterations: 2}, model)
		require.NoError(t, err)

		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, newEchoToolBox(t, &calls), nil))
		// when
		result, err := manager.Run(t.Context(), "explore", "say hi")
		// then
		assert.ErrorIs(t, err, agent.ErrMaxIterations)
		assert.Empty(t, result)
		assert.Len(t, calls, 2)
	})

	t.Run("stops the child when the context is cancelled", func(t *testing.T) {
		// given
		child, _ := newChild(t, answerReply)
		manager := NewManager()
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		// when
		result, err := manager.Run(ctx, "explore", "find the parser")
		// then
		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, result)
	})

	t.Run("reports the run's start, tool calls, token usage and end to the feedback", func(t *testing.T) {
		// given
		var calls []map[string]any
		child, _ := newChild(t, toolCallReply, answerReply)
		feedback := &recordingFeedback{}
		manager := NewManager()
		manager.SetFeedback(feedback)
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, newEchoToolBox(t, &calls), nil))
		// when
		_, err := manager.Run(t.Context(), "explore", "say hi")
		// then
		require.NoError(t, err)
		events, finishErrs := feedback.recorded()
		expected := []string{
			"started explore",
			"tokens explore 15",
			"tool explore echo",
			"tokens explore 30",
			"finished explore",
		}
		assert.Equal(t, expected, events)
		assert.Equal(t, []error{nil}, finishErrs)
	})

	t.Run("reports the cached tokens of each reply to the feedback", func(t *testing.T) {
		// given
		cachedToolCallReply := `{"message":{"content":"","tool_calls":[{"function":{"name":"echo","arguments":{"text":"hi"}}}]},"done":true,"prompt_eval_count":10,"prompt_eval_cached_count":4,"eval_count":5}`
		cachedAnswerReply := `{"message":{"content":"the answer"},"done":true,"prompt_eval_count":20,"prompt_eval_cached_count":8,"eval_count":10}`
		var calls []map[string]any
		child, _ := newChild(t, cachedToolCallReply, cachedAnswerReply)
		feedback := &recordingFeedback{}
		manager := NewManager()
		manager.SetFeedback(feedback)
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, newEchoToolBox(t, &calls), nil))
		expected := []int{4, 8}
		// when
		_, err := manager.Run(t.Context(), "explore", "say hi")
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, feedback.recordedCached())
	})

	t.Run("reports the child's error as the end of a failed run", func(t *testing.T) {
		// given
		child, _ := newChild(t, errorReply)
		feedback := &recordingFeedback{}
		manager := NewManager()
		manager.SetFeedback(feedback)
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
		// when
		_, err := manager.Run(t.Context(), "explore", "find the parser")
		// then
		require.Error(t, err)
		events, finishErrs := feedback.recorded()
		expected := []string{"started explore", "finished explore"}
		assert.Equal(t, expected, events)
		require.Len(t, finishErrs, 1)
		assert.ErrorContains(t, finishErrs[0], "boom")
	})

	t.Run("reports nothing when the run is rejected", func(t *testing.T) {
		tests := map[string]struct {
			name   string
			prompt string
		}{
			"empty prompt": {
				name: "explore",
			},
			"unknown agent": {
				name:   "plan",
				prompt: "find the parser",
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				// given
				child, _ := newChild(t, answerReply)
				feedback := &recordingFeedback{}
				manager := NewManager()
				manager.SetFeedback(feedback)
				require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
				// when
				_, err := manager.Run(t.Context(), tt.name, tt.prompt)
				// then
				require.Error(t, err)
				events, finishErrs := feedback.recorded()
				assert.Empty(t, events)
				assert.Empty(t, finishErrs)
			})
		}
	})

	t.Run("tags the events with the name the run was started with", func(t *testing.T) {
		// given
		var calls []map[string]any
		child, _ := newChild(t, toolCallReply, answerReply, toolCallReply, answerReply)
		toolBox := newEchoToolBox(t, &calls)
		feedback := &recordingFeedback{}
		manager := NewManager()
		manager.SetFeedback(feedback)
		require.NoError(t, manager.Register(Config{Name: "review"}, child, toolBox, nil))
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, toolBox, nil))
		_, err := manager.Run(t.Context(), "review", "say hi")
		require.NoError(t, err)
		// when
		_, err = manager.Run(t.Context(), "explore", "say hi")
		// then
		require.NoError(t, err)
		events, _ := feedback.recorded()
		expected := []string{
			"started review",
			"tokens review 15",
			"tool review echo",
			"tokens review 30",
			"finished review",
			"started explore",
			"tokens explore 15",
			"tool explore echo",
			"tokens explore 30",
			"finished explore",
		}
		assert.Equal(t, expected, events)
	})
}

func TestManagerSetFeedback(t *testing.T) {
	t.Run("ignores a nil feedback and keeps the previous one", func(t *testing.T) {
		// given
		child, _ := newChild(t, answerReply)
		feedback := &recordingFeedback{}
		manager := NewManager()
		manager.SetFeedback(feedback)
		require.NoError(t, manager.Register(Config{Name: "explore"}, child, nil, nil))
		// when
		manager.SetFeedback(nil)
		// then
		_, err := manager.Run(t.Context(), "explore", "find the parser")
		require.NoError(t, err)
		events, _ := feedback.recorded()
		expected := []string{"started explore", "tokens explore 30", "finished explore"}
		assert.Equal(t, expected, events)
	})
}

func TestManagerConcurrentUse(t *testing.T) {
	t.Run("registers, lists, sets feedback and runs from several goroutines", func(t *testing.T) {
		// given
		model, _ := newModelServer(t, answerReply)
		manager := NewManager()
		names := make([]string, 8)
		children := make(map[string]*agent.Agent, len(names))
		for i := range names {
			child, err := agent.New(agent.Config{}, model)
			require.NoError(t, err)

			names[i] = fmt.Sprintf("agent-%d", i)
			children[names[i]] = child
		}
		// when
		var group sync.WaitGroup
		for _, name := range names {
			group.Add(4)

			go func() {
				defer group.Done()
				assert.NoError(t, manager.Register(Config{Name: name}, children[name], nil, nil))
			}()

			go func() {
				defer group.Done()
				manager.SetFeedback(&recordingFeedback{})
			}()

			go func() {
				defer group.Done()
				manager.Agents()
			}()

			go func() {
				defer group.Done()
				_, _ = manager.Run(t.Context(), name, "find the parser")
			}()
		}
		group.Wait()
		// then
		expected := fn.Map(names, func(name string) Info { return Info{Name: name} })
		assert.Equal(t, expected, manager.Agents())
	})
}
