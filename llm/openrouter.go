package llm

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"
)

const (
	openrouterBaseURL        = "https://openrouter.ai/api/v1"
	openrouterChatEndpoint   = "/chat/completions"
	openrouterModelsEndpoint = "/models"
)

type openrouter struct {
	cfg    Config
	client *resty.Client
}

func newOpenRouter(cfg Config) (*openrouter, error) {
	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = openrouterBaseURL
	}

	client := newRestyClient(cfg.BaseURL).
		SetAuthToken(cfg.APIKey)

	return &openrouter{
		cfg:    cfg,
		client: client,
	}, nil
}

func (o *openrouter) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	convertedMessages, err := toORMessages(messages)
	if err != nil {
		return nil, err
	}

	request := orChatRequest{
		Model:     o.cfg.Model,
		Messages:  convertedMessages,
		Tools:     toORTools(tools),
		MaxTokens: o.cfg.MaxTokens,
	}

	var apiResp orChatResponse
	resp, err := o.client.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&apiResp).
		Post(openrouterChatEndpoint)
	if err != nil {
		return nil, fmt.Errorf("openrouter: sending request: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("openrouter: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("openrouter: api error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return nil, errors.New("openrouter: response contained no choices")
	}

	return fromORToAssistantMessage(apiResp)
}

func (o *openrouter) modelInfo(ctx context.Context) (*ModelInfo, error) {
	var apiResp orModelsResponse
	resp, err := o.client.R().
		SetContext(ctx).
		SetResult(&apiResp).
		Get(openrouterModelsEndpoint)
	if err != nil {
		return nil, fmt.Errorf("openrouter: sending request: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("openrouter: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	return fromORToModelInfo(apiResp.Data, o.cfg.Model)
}

func (o *openrouter) changeModel(model string) error {
	o.cfg.Model = model
	return nil
}

func (o *openrouter) currentModel() string {
	return o.cfg.Model
}
