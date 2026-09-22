package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"

	"github.com/jjmrocha/ai-toolkit/internal/command"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpClientName           = "ai-toolkit"
	mcpClientVersion        = "0.1.0"
	defaultToolErrorText    = "MCP tool returned an error"
	defaultResourceMIMEType = "application/octet-stream"
)

type sdkClient struct {
	session   *sdk.ClientSession
	callBacks callBacks
	ready     atomic.Bool
}

type callBacks struct {
	onDisconnect  func(error)
	onToolsChange func()
	onProgress    func(string)
}

var newTransport = func(cfg ClientConfig) sdk.Transport {
	cmd := exec.Command(cfg.Command, cfg.Args...) //nolint:gosec // command and args are operator-provided configuration
	cmd.Env = command.InheritedEnv(cfg.InheritEnv)

	return &sdk.CommandTransport{Command: cmd}
}

func newSDKClient(ctx context.Context, cfg ClientConfig, cb callBacks) (*sdkClient, error) {
	s := &sdkClient{
		callBacks: cb,
	}

	impl := sdk.Implementation{
		Name:    mcpClientName,
		Version: mcpClientVersion,
	}

	options := sdk.ClientOptions{
		ToolListChangedHandler:      s.listChanged,
		ProgressNotificationHandler: s.progressNotification,
	}

	client := sdk.NewClient(&impl, &options)

	session, err := client.Connect(ctx, newTransport(cfg), nil)
	if err != nil {
		return nil, fmt.Errorf("error starting mcp %s: %w", cfg.Name, err)
	}

	s.session = session
	s.ready.Store(true)

	go s.wait()

	return s, nil
}

func (s *sdkClient) close() {
	s.ready.Store(false)
	_ = s.session.Close()
}

func (s *sdkClient) listChanged(_ context.Context, _ *sdk.ToolListChangedRequest) {
	if s.callBacks.onToolsChange != nil && s.ready.Load() {
		go s.callBacks.onToolsChange()
	}
}

func (s *sdkClient) progressNotification(_ context.Context, req *sdk.ProgressNotificationClientRequest) {
	if s.callBacks.onProgress != nil && s.ready.Load() {
		params := req.Params

		if token, ok := params.ProgressToken.(string); ok {
			go s.callBacks.onProgress(token)
		}
	}
}

func (s *sdkClient) wait() {
	err := s.session.Wait()

	if s.callBacks.onDisconnect != nil {
		s.callBacks.onDisconnect(err)
	}
}

func (s *sdkClient) supportsTools() bool {
	res := s.session.InitializeResult()
	if res == nil || res.Capabilities == nil {
		return true
	}

	return res.Capabilities.Tools != nil
}

func (s *sdkClient) listTools(ctx context.Context) ([]toolSpec, error) {
	var tools []toolSpec

	for tool, err := range s.session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}

		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unexpected schema type %T", tool.InputSchema)
		}

		spec := toolSpec{
			name:        tool.Name,
			description: tool.Description,
			schema:      schema,
		}

		tools = append(tools, spec)
	}

	return tools, nil
}

func (s *sdkClient) execute(ctx context.Context, token string, name string, args map[string]any) (string, error) {
	params := sdk.CallToolParams{
		Name:      name,
		Arguments: args,
	}

	if token != "" {
		params.Meta = sdk.Meta{"progressToken": token}
	}

	res, err := s.session.CallTool(ctx, &params)
	if err != nil {
		return "", fmt.Errorf("calling tool %s: %w", name, err)
	}

	if res.IsError {
		return "", fmt.Errorf("tool %s reported an error: %s", name, errorText(res))
	}

	return contentText(res), nil
}

func errorText(res *sdk.CallToolResult) string {
	parts := make([]string, 0, len(res.Content))

	for _, c := range res.Content {
		if t, ok := c.(*sdk.TextContent); ok {
			if text := strings.TrimSpace(t.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}

	if len(parts) == 0 {
		return defaultToolErrorText
	}

	return strings.Join(parts, "\n\n")
}

func contentText(res *sdk.CallToolResult) string {
	parts := make([]string, 0, len(res.Content))

	for _, c := range res.Content {
		switch t := c.(type) {
		case *sdk.TextContent:
			parts = append(parts, t.Text)
		case *sdk.ImageContent:
			parts = append(parts, fmt.Sprintf("[image: %s, %d bytes]", t.MIMEType, len(t.Data)))
		case *sdk.AudioContent:
			parts = append(parts, fmt.Sprintf("[audio: %s, %d bytes]", t.MIMEType, len(t.Data)))
		case *sdk.ResourceLink:
			parts = append(parts, fmt.Sprintf("[resource: %s]", t.URI))
		case *sdk.EmbeddedResource:
			parts = append(parts, embeddedResourceText(t))
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}

	if res.StructuredContent != nil {
		if encoded, err := json.Marshal(res.StructuredContent); err == nil {
			return string(encoded)
		}
	}

	return ""
}

func embeddedResourceText(resource *sdk.EmbeddedResource) string {
	if resource.Resource == nil {
		return "[embedded resource]"
	}

	if resource.Resource.Text != "" {
		return resource.Resource.Text
	}

	mimeType := resource.Resource.MIMEType
	if mimeType == "" {
		mimeType = defaultResourceMIMEType
	}

	return fmt.Sprintf("[embedded resource: %s (%s, %d bytes)]",
		resource.Resource.URI,
		mimeType,
		len(resource.Resource.Blob))
}
