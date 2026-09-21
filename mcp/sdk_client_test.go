package mcp

import (
	"context"
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/fn"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentText(t *testing.T) {
	t.Run("returns the text of a single text block", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: "done"}},
		}
		// when
		result := contentText(res)
		// then
		expected := "done"
		assert.Equal(t, expected, result)
	})

	t.Run("joins several text blocks with a newline", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{
			Content: []sdk.Content{
				&sdk.TextContent{Text: "first"},
				&sdk.TextContent{Text: "second"},
			},
		}
		// when
		result := contentText(res)
		// then
		expected := "first\nsecond"
		assert.Equal(t, expected, result)
	})

	t.Run("describes content the model cannot read as text", func(t *testing.T) {
		testCases := []struct {
			name     string
			content  sdk.Content
			expected string
		}{
			{
				name:     "image",
				content:  &sdk.ImageContent{MIMEType: "image/png", Data: []byte("1234567890")},
				expected: "[image: image/png, 10 bytes]",
			},
			{
				name:     "audio",
				content:  &sdk.AudioContent{MIMEType: "audio/wav", Data: []byte("12345")},
				expected: "[audio: audio/wav, 5 bytes]",
			},
			{
				name:     "resource link",
				content:  &sdk.ResourceLink{URI: "file:///tmp/report.pdf"},
				expected: "[resource: file:///tmp/report.pdf]",
			},
			{
				name: "embedded binary resource",
				content: &sdk.EmbeddedResource{
					Resource: &sdk.ResourceContents{
						URI:      "file:///tmp/logo.png",
						MIMEType: "image/png",
						Blob:     []byte("123"),
					},
				},
				expected: "[embedded resource: file:///tmp/logo.png (image/png, 3 bytes)]",
			},
			{
				name: "embedded binary resource without a media type",
				content: &sdk.EmbeddedResource{
					Resource: &sdk.ResourceContents{
						URI:  "file:///tmp/blob.bin",
						Blob: []byte("12"),
					},
				},
				expected: "[embedded resource: file:///tmp/blob.bin (application/octet-stream, 2 bytes)]",
			},
			{
				name:     "embedded resource with nothing attached",
				content:  &sdk.EmbeddedResource{},
				expected: "[embedded resource]",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				res := &sdk.CallToolResult{Content: []sdk.Content{tc.content}}
				// when
				result := contentText(res)
				// then
				assert.Equal(t, tc.expected, result)
			})
		}
	})

	t.Run("prefers the text of an embedded resource over a descriptor", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{
			Content: []sdk.Content{
				&sdk.EmbeddedResource{
					Resource: &sdk.ResourceContents{
						URI:      "file:///tmp/notes.txt",
						MIMEType: "text/plain",
						Text:     "the notes",
					},
				},
			},
		}
		// when
		result := contentText(res)
		// then
		expected := "the notes"
		assert.Equal(t, expected, result)
	})

	t.Run("keeps text alongside a descriptor", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{
			Content: []sdk.Content{
				&sdk.TextContent{Text: "captured the page"},
				&sdk.ImageContent{MIMEType: "image/png", Data: []byte("1234")},
			},
		}
		// when
		result := contentText(res)
		// then
		expected := "captured the page\n[image: image/png, 4 bytes]"
		assert.Equal(t, expected, result)
	})

	t.Run("falls back to structured content when there is no content block", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{
			StructuredContent: map[string]any{"count": 2},
		}
		// when
		result := contentText(res)
		// then
		expected := `{"count":2}`
		assert.Equal(t, expected, result)
	})

	t.Run("prefers content blocks over structured content", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{
			Content:           []sdk.Content{&sdk.TextContent{Text: "done"}},
			StructuredContent: map[string]any{"count": 2},
		}
		// when
		result := contentText(res)
		// then
		expected := "done"
		assert.Equal(t, expected, result)
	})

	t.Run("returns empty for a result carrying nothing", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{}
		// when
		result := contentText(res)
		// then
		assert.Empty(t, result)
	})
}

func TestErrorText(t *testing.T) {
	t.Run("returns the text the server gave as the reason", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: "file not found"}},
		}
		// when
		result := errorText(res)
		// then
		expected := "file not found"
		assert.Equal(t, expected, result)
	})

	t.Run("joins several text blocks with a blank line", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{
			Content: []sdk.Content{
				&sdk.TextContent{Text: "file not found"},
				&sdk.TextContent{Text: "check the path"},
			},
		}
		// when
		result := errorText(res)
		// then
		expected := "file not found\n\ncheck the path"
		assert.Equal(t, expected, result)
	})

	t.Run("skips blocks holding only whitespace", func(t *testing.T) {
		// given
		res := &sdk.CallToolResult{
			Content: []sdk.Content{
				&sdk.TextContent{Text: "  \n  "},
				&sdk.TextContent{Text: " file not found "},
			},
		}
		// when
		result := errorText(res)
		// then
		expected := "file not found"
		assert.Equal(t, expected, result)
	})

	t.Run("names a reason when the server gave none", func(t *testing.T) {
		testCases := []struct {
			name    string
			content []sdk.Content
		}{
			{name: "no content at all", content: nil},
			{name: "only whitespace", content: []sdk.Content{&sdk.TextContent{Text: "   "}}},
			{
				name:    "only non-text content",
				content: []sdk.Content{&sdk.ImageContent{MIMEType: "image/png", Data: []byte("1")}},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				res := &sdk.CallToolResult{Content: tc.content}
				// when
				result := errorText(res)
				// then
				expected := defaultToolErrorText
				assert.Equal(t, expected, result)
			})
		}
	})
}

type testMCPServer struct {
	server  *sdk.Server
	session *sdk.ServerSession
}

func startTestMCPServer(t *testing.T, toolNames ...string) *testMCPServer {
	t.Helper()

	impl := sdk.Implementation{Name: "test-server", Version: "1.0.0"}
	server := sdk.NewServer(&impl, nil)

	for _, name := range toolNames {
		server.AddTool(testTool(name), testToolHandler)
	}

	testServer := testMCPServer{server: server}

	original := newTransport
	newTransport = func(ClientConfig) sdk.Transport {
		clientSide, serverSide := sdk.NewInMemoryTransports()

		session, err := server.Connect(context.Background(), serverSide, nil)
		require.NoError(t, err)

		testServer.session = session

		return clientSide
	}

	t.Cleanup(func() { newTransport = original })

	return &testServer
}

func testTool(name string) *sdk.Tool {
	tool := sdk.Tool{
		Name:        name,
		Description: name + " tool",
		InputSchema: map[string]any{"type": "object"},
	}

	return &tool
}

func testToolHandler(_ context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
	content := sdk.TextContent{Text: "ok"}
	result := sdk.CallToolResult{Content: []sdk.Content{&content}}

	return &result, nil
}

func registeredToolNames(toolBox *tools.ToolBox) []string {
	return fn.Map(toolBox.Tools(), func(t llm.Tool) string { return t.Name })
}
