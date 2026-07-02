package llm

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

const (
	ollamaBaseURL      = "http://localhost:11434"
	ollamaChatEndpoint = "/api/chat"
	ollamaShowEndpoint = "/api/show"
)

type ollama struct {
	config Config
	client *resty.Client
}

func newOllama(cfg Config) (*ollama, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = ollamaBaseURL
	}

	return &ollama{
		config: cfg,
		client: newRestyClient(cfg.BaseURL),
	}, nil
}

func (o *ollama) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	request := ollamaChatRequest{
		Model:    o.config.Model,
		Messages: toOllamaMessages(messages),
		Tools:    toOllamaTools(tools),
		Stream:   false,
		Think:    toOllamaThink(o.config.Effort),
	}

	if o.config.MaxTokens > 0 {
		request.Options = &ollamaOptions{NumPredict: o.config.MaxTokens}
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

func (o *ollama) modelInfo(ctx context.Context) (*ModelInfo, error) {
	var apiResp ollamaShowResponse
	resp, err := o.client.R().
		SetContext(ctx).
		SetBody(ollamaShowRequest{Model: o.config.Model}).
		SetResult(&apiResp).
		Post(ollamaShowEndpoint)
	if err != nil {
		return nil, fmt.Errorf("ollama: sending request: %w", err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("ollama: %w: %q", ErrModelNotFound, o.config.Model)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	if apiResp.Error != "" {
		return nil, fmt.Errorf("ollama: api error: %s", apiResp.Error)
	}

	return fromOllamaToModelInfo(apiResp, o.config.Model)
}

func (o *ollama) changeModel(model string) error {
	o.config.Model = model
	return nil
}

func (o *ollama) currentModel() string {
	return o.config.Model
}

func (o *ollama) effort() Effort {
	return o.config.Effort
}

func (o *ollama) changeEffort(e Effort) {
	o.config.Effort = e
}
