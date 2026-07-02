package llm

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

const (
	anthropicBaseURL          = "https://api.anthropic.com/v1"
	anthropicMessagesEndpoint = "/messages"
	anthropicModelsEndpoint   = "/models"
	// anthropicVersion is the required anthropic-version header value.
	anthropicVersion = "2023-06-01"
	// defaultMaxTokens is applied when Config.MaxTokens is zero, since the
	// Anthropic Messages API requires a max_tokens on every request.
	defaultMaxTokens = 4096
)

type anthropic struct {
	cfg    Config
	client *resty.Client
}

func newAnthropic(cfg Config) (*anthropic, error) {
	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = anthropicBaseURL
	}

	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = defaultMaxTokens
	}

	client := newRestyClient(cfg.BaseURL).
		SetHeader("x-api-key", cfg.APIKey).
		SetHeader("anthropic-version", anthropicVersion)

	return &anthropic{
		cfg:    cfg,
		client: client,
	}, nil
}

func (a *anthropic) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	request := anthropicChatRequest{
		Model:     a.cfg.Model,
		MaxTokens: a.cfg.MaxTokens,
		System:    toAnthropicSystem(messages),
		Messages:  toAnthropicMessages(messages),
		Tools:     toAnthropicTools(tools),
	}

	var apiResp anthropicChatResponse
	resp, err := a.client.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&apiResp).
		Post(anthropicMessagesEndpoint)
	if err != nil {
		return nil, fmt.Errorf("anthropic: sending request: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("anthropic: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	return fromAnthropicToAssistantMessage(apiResp), nil
}

func (a *anthropic) modelInfo(ctx context.Context) (*ModelInfo, error) {
	var apiResp anthropicModel
	resp, err := a.client.R().
		SetContext(ctx).
		SetResult(&apiResp).
		Get(anthropicModelsEndpoint + "/" + a.cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("anthropic: sending request: %w", err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("anthropic: %w: %q", ErrModelNotFound, a.cfg.Model)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("anthropic: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	return fromAnthropicToModelInfo(apiResp)
}

func (a *anthropic) changeModel(model string) error {
	a.cfg.Model = model
	return nil
}

func (a *anthropic) currentModel() string {
	return a.cfg.Model
}
