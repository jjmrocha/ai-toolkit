package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	chatEndpoint   = "/chat/completions"
	modelsEndpoint = "/models"

	defaultTimeout   = 60 * time.Second
	retryCount       = 5
	retryWaitTime    = 100 * time.Millisecond
	retryMaxWaitTime = 2 * time.Second
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
		cfg.BaseURL = defaultBaseURL
	}

	client := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetAuthToken(cfg.APIKey).
		SetTimeout(defaultTimeout).
		SetRetryCount(retryCount).
		SetRetryWaitTime(retryWaitTime).
		SetRetryMaxWaitTime(retryMaxWaitTime).
		AddRetryCondition(func(r *resty.Response, _ error) bool {
			return r.StatusCode() == http.StatusTooManyRequests || r.StatusCode() >= http.StatusInternalServerError
		})

	return &openrouter{
		cfg:    cfg,
		client: client,
	}, nil
}

func (o *openrouter) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	wireMessages, err := toORMessages(messages)
	if err != nil {
		return nil, err
	}

	request := orRequest{
		Model:    o.cfg.Model,
		Messages: wireMessages,
		Tools:    toORTools(tools),
	}

	var apiResp orResponse
	resp, err := o.client.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&apiResp).
		Post(chatEndpoint)
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

	return toAssistantMessage(apiResp)
}

func (o *openrouter) modelInfo(ctx context.Context) (ModelInfo, error) {
	var apiResp orModelsResponse
	resp, err := o.client.R().
		SetContext(ctx).
		SetResult(&apiResp).
		Get(modelsEndpoint)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("openrouter: sending request: %w", err)
	}

	if resp.IsError() {
		return ModelInfo{}, fmt.Errorf("openrouter: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	return toModelInfo(apiResp.Data, o.cfg.Model)
}
