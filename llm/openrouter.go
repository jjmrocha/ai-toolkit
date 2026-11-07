package llm

import (
	"bytes"
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
	openrouterBaseURL        = "https://openrouter.ai/api/v1"
	openrouterChatEndpoint   = "/chat/completions"
	openrouterModelsEndpoint = "/models"

	openrouterInFlightBudgetExhausted = "in_flight_budget_exhausted"
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

func (o *openrouter) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	convertedMessages, err := toORMessages(messages)
	if err != nil {
		return nil, err
	}

	request := orChatRequest{
		Model:        o.config.Model,
		Messages:     convertedMessages,
		Tools:        toORTools(tools),
		MaxTokens:    o.config.MaxTokens,
		Reasoning:    toORReasoning(o.config.Effort),
		CacheControl: orCacheEphemeral,
		Stream:       true,
	}

	resp, err := o.client.R().
		SetContext(ctx).
		SetBody(request).
		SetDoNotParseResponse(true).
		AddRetryCondition(isORInFlightBudgetExhausted).
		Post(openrouterChatEndpoint)
	if err != nil {
		return nil, fmt.Errorf("openrouter: sending request: %w", err)
	}

	body := rest.WithIdleTimeout(resp.RawBody(), rest.IdleTimeout)
	defer func() { _ = body.Close() }()

	if resp.IsError() {
		message, _ := io.ReadAll(body)
		return nil, fmt.Errorf("openrouter: unexpected status %d: %s", resp.StatusCode(), message)
	}

	apiResp, err := readORStream(body)
	if err != nil {
		return nil, err
	}

	if len(apiResp.Choices) == 0 {
		return nil, errors.New("openrouter: response contained no choices")
	}

	return fromORToAssistantMessage(apiResp)
}

func isORInFlightBudgetExhausted(resp *resty.Response, _ error) bool {
	if resp == nil || resp.StatusCode() != http.StatusPaymentRequired {
		return false
	}

	raw := resp.RawBody()
	message, _ := io.ReadAll(raw)
	_ = raw.Close()
	resp.RawResponse.Body = io.NopCloser(bytes.NewReader(message))

	var apiErr orErrorResponse
	_ = json.Unmarshal(message, &apiErr)
	return apiErr.Error.Metadata.Reason == openrouterInFlightBudgetExhausted
}

func readORStream(body io.Reader) (orChatResponse, error) {
	var (
		content      strings.Builder
		toolCalls    []orToolCall
		finishReason string
		usage        orUsage
		sawChoice    bool
	)

	for data, err := range rest.Events(body) {
		if err != nil {
			return orChatResponse{}, fmt.Errorf("openrouter: reading stream: %w", err)
		}

		if data == "[DONE]" {
			if !sawChoice {
				return orChatResponse{Usage: usage}, nil
			}

			message := orResponseMessage{Content: content.String(), ToolCalls: toolCalls}
			choice := orChoice{Message: message, FinishReason: finishReason}
			return orChatResponse{Choices: []orChoice{choice}, Usage: usage}, nil
		}

		var chunk orStreamChunk
		err = json.Unmarshal([]byte(data), &chunk)
		if err != nil {
			return orChatResponse{}, fmt.Errorf("openrouter: reading stream: %w", err)
		}

		if chunk.Error != nil {
			return orChatResponse{}, fmt.Errorf("openrouter: api error: %s", chunk.Error.Message)
		}

		if chunk.Usage != nil {
			usage = *chunk.Usage
		}

		for _, choice := range chunk.Choices {
			sawChoice = true
			content.WriteString(choice.Delta.Content)

			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}

			toolCalls, err = mergeORToolCalls(toolCalls, choice.Delta.ToolCalls)
			if err != nil {
				return orChatResponse{}, err
			}
		}
	}

	return orChatResponse{}, errors.New("openrouter: stream ended before done")
}

func mergeORToolCalls(toolCalls []orToolCall, fragments []orStreamToolCall) ([]orToolCall, error) {
	for _, fragment := range fragments {
		if fragment.Index < 0 || fragment.Index > len(toolCalls) {
			return nil, fmt.Errorf("openrouter: tool call index %d out of order", fragment.Index)
		}

		if fragment.Index == len(toolCalls) {
			var toolCall orToolCall
			toolCalls = append(toolCalls, toolCall)
		}

		toolCall := &toolCalls[fragment.Index]
		if fragment.ID != "" {
			toolCall.ID = fragment.ID
		}

		if fragment.Type != "" {
			toolCall.Type = fragment.Type
		}

		if fragment.Function.Name != "" {
			toolCall.Function.Name = fragment.Function.Name
		}

		toolCall.Function.Arguments += fragment.Function.Arguments
	}

	return toolCalls, nil
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

	return fromORToModelInfo(apiResp.Data, o.config.Model)
}

func (o *openrouter) changeModel(ctx context.Context, model string) (*ModelInfo, error) {
	oldModel := o.config.Model
	o.config.Model = model

	info, err := o.modelInfo(ctx)
	if err != nil {
		o.config.Model = oldModel
		return nil, err
	}

	return info, nil
}

func (o *openrouter) currentModel() string {
	return o.config.Model
}

func (o *openrouter) effort() Effort {
	return o.config.Effort
}

func (o *openrouter) changeEffort(e Effort) {
	o.config.Effort = e
}
