package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/jjmrocha/ai-toolkit/internal/rest"
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
		client: rest.NewClient(cfg.BaseURL),
	}, nil
}

func (o *ollama) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	request := ollamaChatRequest{
		Model:    o.config.Model,
		Messages: toOllamaMessages(messages),
		Tools:    toOllamaTools(tools),
		Stream:   true,
		Think:    toOllamaThink(o.config.Effort),
	}

	if o.config.MaxTokens > 0 {
		request.Options = &ollamaOptions{NumPredict: o.config.MaxTokens}
	}

	resp, err := o.client.R().
		SetContext(ctx).
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(ollamaChatEndpoint)
	if err != nil {
		return nil, fmt.Errorf("ollama: sending request: %w", err)
	}

	body := rest.WithIdleTimeout(resp.RawBody(), rest.IdleTimeout)
	defer func() { _ = body.Close() }()

	if resp.IsError() {
		message, _ := io.ReadAll(body)
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode(), message)
	}

	apiResp, err := readOllamaStream(body)
	if err != nil {
		return nil, err
	}

	return fromOllamaToAssistantMessage(apiResp), nil
}

func readOllamaStream(body io.Reader) (ollamaChatResponse, error) {
	var (
		content   strings.Builder
		toolCalls []ollamaToolCall
	)

	decoder := json.NewDecoder(body)
	for {
		var chunk ollamaChatResponse
		err := decoder.Decode(&chunk)
		if errors.Is(err, io.EOF) {
			return ollamaChatResponse{}, errors.New("ollama: stream ended before done")
		}

		if err != nil {
			return ollamaChatResponse{}, fmt.Errorf("ollama: reading stream: %w", err)
		}

		if chunk.Error != "" {
			return ollamaChatResponse{}, fmt.Errorf("ollama: api error: %s", chunk.Error)
		}

		content.WriteString(chunk.Message.Content)
		toolCalls = append(toolCalls, chunk.Message.ToolCalls...)

		if chunk.Done {
			chunk.Message.Content = content.String()
			chunk.Message.ToolCalls = toolCalls
			return chunk, nil
		}
	}
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
