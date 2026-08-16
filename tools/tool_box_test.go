package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noopHandler(context.Context, map[string]any) (string, error) { return "", nil }

func TestValidToolName(t *testing.T) {
	t.Run("accepts letters, digits, underscores and hyphens", func(t *testing.T) {
		// when
		result := ValidToolName("get_weather-2")
		// then
		assert.True(t, result)
	})

	t.Run("rejects a name carrying a character the providers reject", func(t *testing.T) {
		// when
		result := ValidToolName("github.create_issue")
		// then
		assert.False(t, result)
	})

	t.Run("rejects an empty name", func(t *testing.T) {
		// when
		result := ValidToolName("")
		// then
		assert.False(t, result)
	})

	t.Run("accepts a name of exactly MaxToolNameLength", func(t *testing.T) {
		// given
		name := strings.Repeat("a", MaxToolNameLength)
		// when
		result := ValidToolName(name)
		// then
		assert.True(t, result)
	})

	t.Run("rejects a name longer than MaxToolNameLength", func(t *testing.T) {
		// given
		name := strings.Repeat("a", MaxToolNameLength+1)
		// when
		result := ValidToolName(name)
		// then
		assert.False(t, result)
	})
}

func TestSanitizeToolName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "leaves an already valid name untouched", input: "get_weather-2", expected: "get_weather-2"},
		{name: "replaces a dot", input: "github.create_issue", expected: "github_create_issue"},
		{name: "replaces a space", input: "get weather", expected: "get_weather"},
		{name: "replaces a multi-byte rune with a single underscore", input: "café", expected: "caf_"},
		{name: "leaves an empty name empty", input: "", expected: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// given
			input := tc.input
			// when
			result := SanitizeToolName(input)
			// then
			assert.Equal(t, tc.expected, result)
		})
	}

	t.Run("does not bound the length", func(t *testing.T) {
		// given: a name well past the provider limit
		input := strings.Repeat(".", MaxToolNameLength+10)
		// when
		result := SanitizeToolName(input)
		// then
		assert.Len(t, result, MaxToolNameLength+10)
		assert.False(t, ValidToolName(result))
	})

	t.Run("produces a name ValidToolName accepts", func(t *testing.T) {
		// given
		input := "github.create issue!"
		// when
		result := SanitizeToolName(input)
		// then
		assert.True(t, ValidToolName(result))
	})
}

func TestToolBoxAdd(t *testing.T) {
	t.Run("registers a tool with a valid name", func(t *testing.T) {
		// given
		box := NewToolBox()
		// when
		err := box.Add(llm.Tool{Name: "srv__echo-1"}, noopHandler)
		// then
		require.NoError(t, err)
		require.Len(t, box.Tools(), 1)
	})

	t.Run("re-registering a name replaces the previous tool", func(t *testing.T) {
		// given
		box := NewToolBox()
		require.NoError(t, box.Add(llm.Tool{Name: "x"}, func(context.Context, map[string]any) (string, error) { return "first", nil }))
		// when
		err := box.Add(llm.Tool{Name: "x"}, func(context.Context, map[string]any) (string, error) { return "second", nil })
		// then
		require.NoError(t, err)
		require.Len(t, box.Tools(), 1)
		result, err := box.Execute(t.Context(), llm.ToolCall{Name: "x"})
		require.NoError(t, err)
		assert.Equal(t, "second", result.Content)
	})

	t.Run("rejects a name with an illegal character and does not register it", func(t *testing.T) {
		// given
		box := NewToolBox()
		// when
		err := box.Add(llm.Tool{Name: "srv.echo"}, noopHandler)
		// then
		assert.ErrorIs(t, err, ErrInvalidToolName)
		assert.Empty(t, box.Tools())
	})

	t.Run("rejects an empty name", func(t *testing.T) {
		// given
		box := NewToolBox()
		// when
		err := box.Add(llm.Tool{Name: ""}, noopHandler)
		// then
		assert.ErrorIs(t, err, ErrInvalidToolName)
	})

	t.Run("accepts a name of exactly MaxToolNameLength", func(t *testing.T) {
		// given
		box := NewToolBox()
		longName := strings.Repeat("a", MaxToolNameLength)
		// when
		err := box.Add(llm.Tool{Name: longName}, noopHandler)
		// then
		require.NoError(t, err)
	})

	t.Run("rejects a name longer than MaxToolNameLength", func(t *testing.T) {
		// given
		box := NewToolBox()
		longName := strings.Repeat("a", MaxToolNameLength+1)
		// when
		err := box.Add(llm.Tool{Name: longName}, noopHandler)
		// then
		assert.ErrorIs(t, err, ErrInvalidToolName)
	})

	t.Run("rejects a nil handler and does not register it", func(t *testing.T) {
		// given
		box := NewToolBox()
		// when
		err := box.Add(llm.Tool{Name: "echo"}, nil)
		// then
		assert.ErrorIs(t, err, ErrNilHandler)
		assert.Empty(t, box.Tools())
	})
}

func TestToolBoxRemove(t *testing.T) {
	t.Run("removes a registered tool", func(t *testing.T) {
		// given
		box := NewToolBox()
		require.NoError(t, box.Add(llm.Tool{Name: "a"}, noopHandler))
		// when
		box.Remove("a")
		// then
		assert.Empty(t, box.Tools())
		_, err := box.Execute(t.Context(), llm.ToolCall{Name: "a"})
		assert.ErrorIs(t, err, ErrToolNotFound)
	})

	t.Run("is a no-op for an unknown tool", func(t *testing.T) {
		// given
		box := NewToolBox()
		require.NoError(t, box.Add(llm.Tool{Name: "a"}, noopHandler))
		// when
		box.Remove("ghost")
		// then: the tools that are registered stay registered
		result := box.Tools()
		require.Len(t, result, 1)
		assert.Equal(t, "a", result[0].Name)
	})
}

func TestToolBoxTools(t *testing.T) {
	t.Run("returns every registered tool definition", func(t *testing.T) {
		// given
		box := NewToolBox()
		require.NoError(t, box.Add(llm.Tool{Name: "a"}, noopHandler))
		require.NoError(t, box.Add(llm.Tool{Name: "b"}, noopHandler))
		// when
		result := box.Tools()
		// then
		require.Len(t, result, 2)
		assert.ElementsMatch(t, []string{"a", "b"}, []string{result[0].Name, result[1].Name})
	})

	t.Run("returns the tools sorted by name", func(t *testing.T) {
		// given: registration order differs from name order
		box := NewToolBox()
		require.NoError(t, box.Add(llm.Tool{Name: "zeta"}, noopHandler))
		require.NoError(t, box.Add(llm.Tool{Name: "alpha"}, noopHandler))
		require.NoError(t, box.Add(llm.Tool{Name: "mid"}, noopHandler))
		// when
		result := box.Tools()
		// then: stable order keeps provider prompt prefixes cacheable
		expected := []string{"alpha", "mid", "zeta"}
		names := []string{result[0].Name, result[1].Name, result[2].Name}
		assert.Equal(t, expected, names)
	})

	t.Run("returns nothing for an empty box", func(t *testing.T) {
		// given
		box := NewToolBox()
		// when
		result := box.Tools()
		// then
		assert.Empty(t, result)
	})
}

func TestToolBoxConcurrentAccess(t *testing.T) {
	// given: correctness here is enforced by the race detector
	const goroutines = 50

	box := NewToolBox()
	require.NoError(t, box.Add(llm.Tool{Name: "stable"}, noopHandler))

	var wg sync.WaitGroup
	// when
	for i := range goroutines {
		name := fmt.Sprintf("tool-%d", i)

		wg.Go(func() {
			_ = box.Add(llm.Tool{Name: name}, noopHandler)
			box.Remove(name)
		})
		wg.Go(func() {
			_ = box.Tools()
			_, _ = box.Execute(t.Context(), llm.ToolCall{Name: "stable"})
		})
	}

	wg.Wait()
	// then: the tool that was never removed is still registered and runnable
	result, err := box.Execute(t.Context(), llm.ToolCall{Name: "stable"})
	require.NoError(t, err)
	assert.Equal(t, "stable", result.ToolName)
	assert.Len(t, box.Tools(), 1)
}

func TestToolBoxExecute(t *testing.T) {
	t.Run("runs the handler and wraps its result", func(t *testing.T) {
		// given
		box := NewToolBox()
		var gotArgs map[string]any
		require.NoError(t, box.Add(llm.Tool{Name: "echo"}, func(_ context.Context, args map[string]any) (string, error) {
			gotArgs = args
			return "sunny", nil
		}))
		call := llm.ToolCall{ID: "call_1", Name: "echo", Arguments: map[string]any{"city": "Lisbon"}}
		// when
		result, err := box.Execute(t.Context(), call)
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
		result, err := box.Execute(t.Context(), llm.ToolCall{Name: "missing"})
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrToolNotFound)
	})

	t.Run("wraps the handler error", func(t *testing.T) {
		// given
		box := NewToolBox()
		expectedErr := errors.New("boom")
		require.NoError(t, box.Add(llm.Tool{Name: "fail"}, func(context.Context, map[string]any) (string, error) {
			return "", expectedErr
		}))
		// when
		result, err := box.Execute(t.Context(), llm.ToolCall{Name: "fail"})
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("passes the caller's context to the handler", func(t *testing.T) {
		// given
		type ctxKey string
		const key ctxKey = "k"
		box := NewToolBox()
		var got any
		require.NoError(t, box.Add(llm.Tool{Name: "peek"}, func(ctx context.Context, _ map[string]any) (string, error) {
			got = ctx.Value(key)
			return "ok", nil
		}))
		ctx := context.WithValue(t.Context(), key, "v")
		// when
		_, err := box.Execute(ctx, llm.ToolCall{Name: "peek"})
		// then
		require.NoError(t, err)
		assert.Equal(t, "v", got)
	})
}
