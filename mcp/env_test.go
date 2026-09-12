package mcp

import (
	"fmt"
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envReportingServer(t *testing.T, name string, variable string, inherit []string) *tools.ToolBox {
	t.Helper()

	initResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":%q}}`, protocolVersion)
	listResp := `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"env","description":"Env","inputSchema":{"type":"object"}}]}}`
	script := fmt.Sprintf(
		`while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*) echo '%s';;
    *'"method":"tools/list"'*) echo '%s';;
    *'"method":"tools/call"'*) printf '{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"%%s"}]}}\n' "${%s:-absent}";;
  esac
done`,
		initResp, listResp, variable,
	)

	tb := tools.NewToolBox()
	client, err := NewClient(t.Context(), ClientConfig{
		Name:       name,
		Command:    "sh",
		Args:       []string{"-c", script},
		InheritEnv: inherit,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.RegisterTools(t.Context(), tb))

	return tb
}

func TestClientInheritEnv(t *testing.T) {
	t.Run("keeps a credential out of the server's environment", func(t *testing.T) {
		// given
		t.Setenv("ANTHROPIC_API_KEY", "sk-secret")
		tb := envReportingServer(t, "srv", "ANTHROPIC_API_KEY", nil)
		// when
		result, err := tb.Execute(t.Context(), llm.ToolCall{Name: "srv__env"})
		// then
		require.NoError(t, err)
		expected := "absent"
		assert.Equal(t, expected, result.Content)
	})

	t.Run("passes a variable the config names", func(t *testing.T) {
		// given
		t.Setenv("GITHUB_TOKEN", "ghp-secret")
		tb := envReportingServer(t, "srv", "GITHUB_TOKEN", []string{"GITHUB_TOKEN"})
		// when
		result, err := tb.Execute(t.Context(), llm.ToolCall{Name: "srv__env"})
		// then
		require.NoError(t, err)
		expected := "ghp-secret"
		assert.Equal(t, expected, result.Content)
	})

	t.Run("passes the default set without being asked", func(t *testing.T) {
		// given
		t.Setenv("HTTPS_PROXY", "http://proxy:3128")
		tb := envReportingServer(t, "srv", "HTTPS_PROXY", nil)
		// when
		result, err := tb.Execute(t.Context(), llm.ToolCall{Name: "srv__env"})
		// then
		require.NoError(t, err)
		expected := "http://proxy:3128"
		assert.Equal(t, expected, result.Content)
	})
}
