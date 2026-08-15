package llm

import (
	"fmt"
	"strings"

	"github.com/jjmrocha/go-algo/fn"
)

func toAnthropicSystem(messages []Message) string {
	var parts []string

	for _, m := range messages {
		if m.Role() == SystemRole {
			parts = append(parts, messageValue[SystemMessage](m).Content)
		}
	}

	return strings.Join(parts, "\n\n")
}

var cacheEphemeral = &anthropicCacheControl{Type: "ephemeral"}

func toAnthropicSystemBlocks(messages []Message) []anthropicSystemBlock {
	text := toAnthropicSystem(messages)
	if text == "" {
		return nil
	}

	return []anthropicSystemBlock{{Type: "text", Text: text, CacheControl: cacheEphemeral}}
}

func toAnthropicMessages(messages []Message) []anthropicMessage {
	var result []anthropicMessage

	appendBlocks := func(role string, blocks []anthropicContentBlock) {
		if n := len(result); n > 0 && result[n-1].Role == role {
			result[n-1].Content = append(result[n-1].Content, blocks...)
			return
		}

		anthropicMsg := anthropicMessage{Role: role, Content: blocks}
		result = append(result, anthropicMsg)
	}

	for _, m := range messages {
		switch m.Role() {
		case UserRole:
			msg := messageValue[UserMessage](m)
			block := anthropicContentBlock{Type: "text", Text: msg.Content}
			appendBlocks(string(UserRole), []anthropicContentBlock{block})
		case AssistantRole:
			msg := messageValue[AssistantMessage](m)

			if blocks, ok := msg.raw.([]anthropicContentBlock); ok {
				appendBlocks(string(AssistantRole), blocks)
				break
			}

			var blocks []anthropicContentBlock
			if msg.Content != "" {
				block := anthropicContentBlock{Type: "text", Text: msg.Content}
				blocks = append(blocks, block)
			}

			for _, call := range msg.ToolCalls {
				input := call.Arguments
				if input == nil {
					input = map[string]any{}
				}

				block := anthropicContentBlock{
					Type:  "tool_use",
					ID:    call.ID,
					Name:  call.Name,
					Input: input,
				}
				blocks = append(blocks, block)
			}

			appendBlocks(string(AssistantRole), blocks)
		case ToolRole:
			msg := messageValue[ToolMessage](m)
			block := anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			}
			appendBlocks(string(UserRole), []anthropicContentBlock{block})
		}
	}

	return result
}

func toAnthropicThinking(e Effort) *anthropicThinking {
	if e == EffortOff {
		return nil
	}

	return &anthropicThinking{Type: "enabled", BudgetTokens: e.tokenBudget()}
}

func toAnthropicTools(tools []Tool) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}

	toolList := fn.Map(tools, func(t Tool) anthropicTool {
		return anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Schema,
		}
	})

	toolList[len(toolList)-1].CacheControl = cacheEphemeral

	return toolList
}

func fromAnthropicToAssistantMessage(resp anthropicChatResponse) *AssistantMessage {
	promptTokens := resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens + resp.Usage.CacheReadInputTokens

	result := AssistantMessage{
		StopReason: resp.StopReason,
		Stats: Stats{
			PromptTokens:     promptTokens,
			OutputTokens:     resp.Usage.OutputTokens,
			TotalTokens:      promptTokens + resp.Usage.OutputTokens,
			CacheWriteTokens: resp.Usage.CacheCreationInputTokens,
			CacheReadTokens:  resp.Usage.CacheReadInputTokens,
		},
		raw: resp.Content,
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			toolCall := ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			}
			result.ToolCalls = append(result.ToolCalls, toolCall)
		}
	}

	return &result
}

func fromAnthropicToModelInfo(model anthropicModel, id string) (*ModelInfo, error) {
	if model.MaxInputTokens == 0 {
		return nil, fmt.Errorf("anthropic: %w: %q", ErrMissingContextLength, id)
	}

	return &ModelInfo{
		Name:        id,
		ContextSize: model.MaxInputTokens,
	}, nil
}
