package llm

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

const (
	defaultOllamaBaseURL = "http://localhost:11434"
	ollamaChatEndpoint   = "/api/chat"
	ollamaShowEndpoint   = "/api/show"
)

type ollama struct {
	cfg    Config
	client *resty.Client
}

func newOllama(cfg Config) (*ollama, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultOllamaBaseURL
	}

	return &ollama{
		cfg:    cfg,
		client: newRestyClient(cfg.BaseURL),
	}, nil
}

func (o *ollama) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	request := ollamaChatRequest{
		Model:    o.cfg.Model,
		Messages: toOllamaMessages(messages),
		Tools:    toOllamaTools(tools),
		Stream:   false,
	}

	var apiResp ollamaChatResponse
	resp, err := o.client.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&apiResp).
		Post(ollamaChatEndpoint)
	if err != nil {
		return nil, fmt.Errorf("ollama: sending request: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	if apiResp.Error != "" {
		return nil, fmt.Errorf("ollama: api error: %s", apiResp.Error)
	}

	return fromOllamaToAssistantMessage(apiResp), nil
}

func (o *ollama) modelInfo(ctx context.Context) (ModelInfo, error) {
	var apiResp ollamaShowResponse
	resp, err := o.client.R().
		SetContext(ctx).
		SetBody(ollamaShowRequest{Model: o.cfg.Model}).
		SetResult(&apiResp).
		Post(ollamaShowEndpoint)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("ollama: sending request: %w", err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return ModelInfo{}, fmt.Errorf("ollama: %w: %q", ErrModelNotFound, o.cfg.Model)
	}

	if resp.IsError() {
		return ModelInfo{}, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	if apiResp.Error != "" {
		return ModelInfo{}, fmt.Errorf("ollama: api error: %s", apiResp.Error)
	}

	return fromOllamaToModelInfo(apiResp, o.cfg.Model), nil
}
