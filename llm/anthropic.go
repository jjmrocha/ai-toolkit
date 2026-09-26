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
	anthropicBaseURL          = "https://api.anthropic.com/v1"
	anthropicMessagesEndpoint = "/messages"
	anthropicModelsEndpoint   = "/models"
	anthropicVersion          = "2023-06-01"
	defaultMaxTokens          = 4096
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

	client := rest.NewClient(cfg.BaseURL).
		SetHeader("x-api-key", cfg.APIKey).
		SetHeader("anthropic-version", anthropicVersion)

	return &anthropic{
		config: cfg,
		client: client,
	}, nil
}

func (a *anthropic) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	request := anthropicChatRequest{
		Model:        a.config.Model,
		MaxTokens:    a.config.MaxTokens,
		System:       toAnthropicSystemBlocks(messages),
		Messages:     toAnthropicMessages(messages),
		Tools:        toAnthropicTools(tools),
		Thinking:     thinkingAdaptive,
		OutputConfig: toAnthropicOutputConfig(a.config.Effort),
		Stream:       true,
	}

	resp, err := a.client.R().
		SetContext(ctx).
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(anthropicMessagesEndpoint)
	if err != nil {
		return nil, fmt.Errorf("anthropic: sending request: %w", err)
	}

	body := rest.WithIdleTimeout(resp.RawBody(), rest.IdleTimeout)
	defer func() { _ = body.Close() }()

	if resp.IsError() {
		message, _ := io.ReadAll(body)
		return nil, fmt.Errorf("anthropic: unexpected status %d: %s", resp.StatusCode(), message)
	}

	apiResp, err := readAnthropicStream(body)
	if err != nil {
		return nil, err
	}

	return fromAnthropicToAssistantMessage(apiResp), nil
}

func readAnthropicStream(body io.Reader) (anthropicChatResponse, error) {
	stream := anthropicStream{parts: make(map[int]*strings.Builder)}

	for data, err := range rest.Events(body) {
		if err != nil {
			return anthropicChatResponse{}, fmt.Errorf("anthropic: reading stream: %w", err)
		}

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return anthropicChatResponse{}, fmt.Errorf("anthropic: reading stream: %w", err)
		}

		done, err := stream.apply(event)
		if err != nil {
			return anthropicChatResponse{}, err
		}

		if done {
			return stream.response, nil
		}
	}

	return anthropicChatResponse{}, errors.New("anthropic: stream ended before done")
}

type anthropicStream struct {
	response anthropicChatResponse
	parts    map[int]*strings.Builder
}

func (s *anthropicStream) apply(event anthropicStreamEvent) (bool, error) {
	switch event.Type {
	case "message_start":
		if event.Message != nil {
			s.response.Usage = event.Message.Usage
		}
	case "content_block_start":
		if event.ContentBlock == nil || event.Index != len(s.response.Content) {
			return false, fmt.Errorf("anthropic: content block index %d out of order", event.Index)
		}

		s.response.Content = append(s.response.Content, *event.ContentBlock)
		s.parts[event.Index] = &strings.Builder{}
	case "content_block_delta":
		if !s.started(event.Index) {
			return false, fmt.Errorf("anthropic: content block index %d not started", event.Index)
		}

		s.applyDelta(event.Index, event.Delta)
	case "content_block_stop":
		if !s.started(event.Index) {
			return false, fmt.Errorf("anthropic: content block index %d not started", event.Index)
		}

		return false, s.finishBlock(event.Index)
	case "message_delta":
		s.response.StopReason = event.Delta.StopReason
		s.response.Usage.OutputTokens = event.Usage.OutputTokens
	case "message_stop":
		return true, nil
	case "error":
		if event.Error == nil {
			return false, errors.New("anthropic: api error")
		}

		return false, fmt.Errorf("anthropic: api error: %s", event.Error.Message)
	}

	return false, nil
}

func (s *anthropicStream) started(index int) bool {
	return index >= 0 && index < len(s.response.Content)
}

func (s *anthropicStream) applyDelta(index int, delta anthropicStreamDelta) {
	part := s.parts[index]

	switch delta.Type {
	case "text_delta":
		part.WriteString(delta.Text)
	case "thinking_delta":
		part.WriteString(delta.Thinking)
	case "input_json_delta":
		part.WriteString(delta.PartialJSON)
	case "signature_delta":
		s.response.Content[index].Signature += delta.Signature
	}
}

func (s *anthropicStream) finishBlock(index int) error {
	block := &s.response.Content[index]
	part := s.parts[index].String()

	switch block.Type {
	case typeText:
		block.Text += part
	case "thinking":
		block.Thinking += part
	case typeToolUse:
		if part == "" {
			return nil
		}

		var input map[string]any
		if err := json.Unmarshal([]byte(part), &input); err != nil {
			return fmt.Errorf("anthropic: decoding tool use input: %w", err)
		}

		block.Input = input
	}

	return nil
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

	return fromAnthropicToModelInfo(apiResp, a.config.Model)
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
