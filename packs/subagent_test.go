package packs

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jjmrocha/ai-toolkit/agent"
	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/subagent"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/fn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	subagentAnswerReply = `{"message":{"content":"the answer"},"done":true}`
	subagentEmptyReply  = `{"message":{"content":""},"done":true}`
	subagentModelInfo   = `{"model_info":{"general.architecture":"test","test.context_length":8192}}`
)

func newSubAgentChild(t *testing.T, reply string) *agent.Agent {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path != "/api/chat" {
			_, _ = io.WriteString(w, subagentModelInfo)
			return
		}

		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(server.Close)

	model, err := llm.New(llm.Config{Provider: llm.ProviderOllama, Model: "test", BaseURL: server.URL})
	require.NoError(t, err)

	child, err := agent.New(agent.Config{}, model)
	require.NoError(t, err)

	return child
}

func newSubAgentManager(t *testing.T, reply string) *subagent.Manager {
	t.Helper()

	child := newSubAgentChild(t, reply)
	manager := subagent.NewManager()

	require.NoError(t, manager.Register(subagent.Config{Name: "review", Description: "Reviews code"}, child, nil, nil))
	require.NoError(t, manager.Register(subagent.Config{Name: "plain"}, child, nil, nil))
	require.NoError(t, manager.Register(subagent.Config{Name: "explore", Description: "Finds code"}, child, nil, nil))

	return manager
}

func runSubAgentTool(t *testing.T, manager *subagent.Manager, args map[string]any) (string, error) {
	t.Helper()

	toolBox := tools.NewToolBox()

	pack, err := SubAgentTools(toolBox, manager)
	require.NoError(t, err)

	defer func() { _ = pack.Close() }()

	call := llm.ToolCall{ID: "call-1", Name: subagentToolName, Arguments: args}

	message, err := toolBox.Execute(t.Context(), call)
	if err != nil {
		return "", err
	}

	return message.Content, nil
}

func TestSubAgentToolsInstructions(t *testing.T) {
	t.Run("returns no instructions", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, subagentAnswerReply)
		pack, err := SubAgentTools(tools.NewToolBox(), manager)
		require.NoError(t, err)
		t.Cleanup(func() { _ = pack.Close() })
		// when
		result := pack.Instructions(t.Context())
		// then
		assert.Nil(t, result)
	})
}

func TestSubAgentTools(t *testing.T) {
	t.Run("registers the subagent tool", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, subagentAnswerReply)
		toolBox := tools.NewToolBox()
		// when
		pack, err := SubAgentTools(toolBox, manager)
		require.NoError(t, err)

		defer func() { _ = pack.Close() }()
		// then
		expected := []string{subagentToolName}
		result := fn.Map(toolBox.Tools(), func(tool llm.Tool) string { return tool.Name })
		assert.Equal(t, expected, result)
	})

	t.Run("lists the manager's agents in the tool description, sorted by name", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, subagentAnswerReply)
		toolBox := tools.NewToolBox()
		pack, err := SubAgentTools(toolBox, manager)
		require.NoError(t, err)

		defer func() { _ = pack.Close() }()
		// when
		result, ok := toolBox.Tool(subagentToolName)
		// then
		require.True(t, ok)
		expected := subagentDescription + "\n\nAvailable subagents:\n" +
			"- explore: Finds code\n" +
			"- plain\n" +
			"- review: Reviews code"
		assert.Equal(t, expected, result.Description)
	})

	t.Run("keeps the description it was created with when an agent is registered later", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, subagentAnswerReply)
		toolBox := tools.NewToolBox()
		pack, err := SubAgentTools(toolBox, manager)
		require.NoError(t, err)

		defer func() { _ = pack.Close() }()

		late := newSubAgentChild(t, subagentAnswerReply)
		require.NoError(t, manager.Register(subagent.Config{Name: "late"}, late, nil, nil))
		// when
		result, ok := toolBox.Tool(subagentToolName)
		// then
		require.True(t, ok)
		assert.NotContains(t, result.Description, "late")
	})

	t.Run("returns ErrNoAgents and registers nothing when the manager has no agents", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()
		// when
		result, err := SubAgentTools(toolBox, subagent.NewManager())
		// then
		require.ErrorIs(t, err, ErrNoAgents)
		assert.Nil(t, result)
		assert.Empty(t, toolBox.Tools())
	})

	t.Run("removes the tool on close", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, subagentAnswerReply)
		toolBox := tools.NewToolBox()
		pack, err := SubAgentTools(toolBox, manager)
		require.NoError(t, err)
		// when
		require.NoError(t, pack.Close())
		// then
		assert.Empty(t, toolBox.Tools())
		assert.NoError(t, pack.Close())
	})

	t.Run("leaves a newer pack's tool in place on a second close", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, subagentAnswerReply)
		toolBox := tools.NewToolBox()
		oldPack, err := SubAgentTools(toolBox, manager)
		require.NoError(t, err)
		require.NoError(t, oldPack.Close())
		newPack, err := SubAgentTools(toolBox, manager)
		require.NoError(t, err)

		defer func() { _ = newPack.Close() }()
		// when
		err = oldPack.Close()
		// then
		require.NoError(t, err)
		expected := []string{subagentToolName}
		result := fn.Map(toolBox.Tools(), func(tool llm.Tool) string { return tool.Name })
		assert.Equal(t, expected, result)
	})

	t.Run("returns the child's final answer", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, subagentAnswerReply)
		args := map[string]any{"agent": "explore", "prompt": "find the parser"}
		// when
		result, err := runSubAgentTool(t, manager, args)
		// then
		require.NoError(t, err)
		assert.Equal(t, "the answer", result)
	})

	t.Run("says so when the child ends without text", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, subagentEmptyReply)
		args := map[string]any{"agent": "explore", "prompt": "find the parser"}
		// when
		result, err := runSubAgentTool(t, manager, args)
		// then
		require.NoError(t, err)
		assert.Equal(t, subagentNoText, result)
	})

	t.Run("rejects a call with a missing or mistyped argument", func(t *testing.T) {
		tests := map[string]struct {
			args     map[string]any
			expected error
		}{
			"no agent": {
				args:     map[string]any{"prompt": "find the parser"},
				expected: tools.ErrFieldNotFound,
			},
			"no prompt": {
				args:     map[string]any{"agent": "explore"},
				expected: tools.ErrFieldNotFound,
			},
			"agent is not a string": {
				args:     map[string]any{"agent": 1.0, "prompt": "find the parser"},
				expected: tools.ErrInvalidFieldType,
			},
			"prompt is not a string": {
				args:     map[string]any{"agent": "explore", "prompt": 1.0},
				expected: tools.ErrInvalidFieldType,
			},
			"empty prompt": {
				args:     map[string]any{"agent": "explore", "prompt": ""},
				expected: subagent.ErrNoPrompt,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				// given
				manager := newSubAgentManager(t, subagentAnswerReply)
				// when
				result, err := runSubAgentTool(t, manager, tt.args)
				// then
				assert.ErrorIs(t, err, tt.expected)
				assert.Empty(t, result)
			})
		}
	})

	t.Run("names the missing agent when the agent is unknown", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, subagentAnswerReply)
		args := map[string]any{"agent": "plan", "prompt": "find the parser"}
		// when
		result, err := runSubAgentTool(t, manager, args)
		// then
		require.ErrorIs(t, err, subagent.ErrAgentNotFound)
		assert.ErrorContains(t, err, "agent not found: plan")
		assert.Empty(t, result)
	})

	t.Run("returns the child's error", func(t *testing.T) {
		// given
		manager := newSubAgentManager(t, `{"error":"boom"}`)
		args := map[string]any{"agent": "explore", "prompt": "find the parser"}
		// when
		result, err := runSubAgentTool(t, manager, args)
		// then
		assert.ErrorContains(t, err, "boom")
		assert.Empty(t, result)
	})
}
