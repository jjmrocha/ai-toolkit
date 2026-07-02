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
	config Config
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
		config: cfg,
		client: client,
	}, nil
}

func (a *anthropic) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	request := anthropicChatRequest{
		Model:     a.config.Model,
		MaxTokens: a.config.MaxTokens + a.config.Effort.tokenBudget(),
		System:    toAnthropicSystem(messages),
		Messages:  toAnthropicMessages(messages),
		Tools:     toAnthropicTools(tools),
		Thinking:  toAnthropicThinking(a.config.Effort),
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
		Get(anthropicModelsEndpoint + "/" + a.config.Model)
	if err != nil {
		return nil, fmt.Errorf("anthropic: sending request: %w", err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("anthropic: %w: %q", ErrModelNotFound, a.config.Model)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("anthropic: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	return fromAnthropicToModelInfo(apiResp)
}

func (a *anthropic) changeModel(model string) error {
	a.config.Model = model
	return nil
}

func (a *anthropic) currentModel() string {
	return a.config.Model
}

func (a *anthropic) effort() Effort {
	return a.config.Effort
}

func (a *anthropic) changeEffort(e Effort) {
	a.config.Effort = e
}
