package decision

import (
	"context"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/jjmrocha/ai-toolkit/internal/rest"
)

const (
	openrouterBaseURL          = "https://openrouter.ai/api/alpha"
	openrouterDecisionEndpoint = "/decisions"
)

type openrouter struct {
	config Config
	client *resty.Client
}

func newOpenRouter(cfg Config) (*openrouter, error) {
	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = openrouterBaseURL
	}

	client := rest.NewClient(cfg.BaseURL).
		SetAuthToken(cfg.APIKey)

	return &openrouter{
		config: cfg,
		client: client,
	}, nil
}

func (o *openrouter) ask(ctx context.Context, req Request) (*Response, error) {
	request := orDecisionRequest{
		Model:     o.config.Model,
		State:     req.State,
		Questions: toORQuestions(req.Questions),
	}

	var apiResp orDecisionResponse
	resp, err := o.client.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&apiResp).
		Post(openrouterDecisionEndpoint)
	if err != nil {
		return nil, fmt.Errorf("openrouter: sending request: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("openrouter: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("openrouter: api error: %s", apiResp.Error.Message)
	}

	return fromORToResponse(apiResp, req.Questions)
}

func (o *openrouter) currentModel() string {
	return o.config.Model
}
