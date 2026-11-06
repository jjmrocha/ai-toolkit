package decision

import (
	"context"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/jjmrocha/ai-toolkit/internal/rest"
)

const (
	ollamaBaseURL          = "http://localhost:11434"
	ollamaDecisionEndpoint = "/v1/systemone"
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
		client: rest.NewClient(cfg.BaseURL),
	}, nil
}

func (o *ollama) ask(ctx context.Context, req Request) (*Response, error) {
	request := ollamaDecisionRequest{
		Model:     o.config.Model,
		State:     req.State,
		Questions: toOllamaQuestions(req.Questions),
	}

	var apiResp ollamaDecisionResponse
	resp, err := o.client.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&apiResp).
		Post(ollamaDecisionEndpoint)
	if err != nil {
		return nil, fmt.Errorf("ollama: sending request: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	return fromOllamaToResponse(apiResp, req.Questions)
}

func (o *ollama) currentModel() string {
	return o.config.Model
}
