package tools

import (
	"errors"
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noopHandler(map[string]any) (string, error) { return "", nil }

func TestAddTool(t *testing.T) {
	t.Run("re-registering a name replaces the previous tool", func(t *testing.T) {
		// given
		box := NewToolBox()
		box.AddTool(llm.Tool{Name: "x"}, func(map[string]any) (string, error) { return "first", nil })
		// when
		box.AddTool(llm.Tool{Name: "x"}, func(map[string]any) (string, error) { return "second", nil })
		// then
		require.Len(t, box.GetTools(), 1)
		result, err := box.ExecuteTool(llm.ToolCall{Name: "x"})
		require.NoError(t, err)
		assert.Equal(t, "second", result.Content)
	})
}

func TestRemoveTool(t *testing.T) {
	t.Run("removes a registered tool", func(t *testing.T) {
		// given
		box := NewToolBox()
		box.AddTool(llm.Tool{Name: "a"}, noopHandler)
		// when
		box.RemoveTool("a")
		// then
		assert.Empty(t, box.GetTools())
		_, err := box.ExecuteTool(llm.ToolCall{Name: "a"})
		assert.ErrorIs(t, err, ErrToolNotFound)
	})

	t.Run("is a no-op for an unknown tool", func(t *testing.T) {
		// given
		box := NewToolBox()
		// then
		assert.NotPanics(t, func() { box.RemoveTool("ghost") })
	})
}

func TestGetTools(t *testing.T) {
	t.Run("returns every registered tool definition", func(t *testing.T) {
		// given
		box := NewToolBox()
		box.AddTool(llm.Tool{Name: "a"}, noopHandler)
		box.AddTool(llm.Tool{Name: "b"}, noopHandler)
		// when
		result := box.GetTools()
		// then
		require.Len(t, result, 2)
		assert.ElementsMatch(t, []string{"a", "b"}, []string{result[0].Name, result[1].Name})
	})

	t.Run("returns nothing for an empty box", func(t *testing.T) {
		// given
		box := NewToolBox()
		// when
		result := box.GetTools()
		// then
		assert.Empty(t, result)
	})
}

func TestExecuteTool(t *testing.T) {
	t.Run("runs the handler and wraps its result", func(t *testing.T) {
		// given
		box := NewToolBox()
		var gotArgs map[string]any
		box.AddTool(llm.Tool{Name: "echo"}, func(args map[string]any) (string, error) {
			gotArgs = args
			return "sunny", nil
		})
		call := llm.ToolCall{ID: "call_1", Name: "echo", Arguments: map[string]any{"city": "Lisbon"}}
		// when
		result, err := box.ExecuteTool(call)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "call_1", result.ToolCallID)
		assert.Equal(t, "echo", result.ToolName)
		assert.Equal(t, "sunny", result.Content)
		assert.Equal(t, map[string]any{"city": "Lisbon"}, gotArgs)
	})

	t.Run("returns ErrToolNotFound for an unknown tool", func(t *testing.T) {
		// given
		box := NewToolBox()
		// when
		result, err := box.ExecuteTool(llm.ToolCall{Name: "missing"})
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrToolNotFound)
	})

	t.Run("wraps the handler error", func(t *testing.T) {
		// given
		box := NewToolBox()
		expectedErr := errors.New("boom")
		box.AddTool(llm.Tool{Name: "fail"}, func(map[string]any) (string, error) {
			return "", expectedErr
		})
		// when
		result, err := box.ExecuteTool(llm.ToolCall{Name: "fail"})
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, expectedErr)
	})
}
