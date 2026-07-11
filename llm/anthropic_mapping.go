package llm

import (
	"fmt"
	"strings"
)

// toAnthropicSystem collects the content of every [SystemMessage] into the
// single top-level system prompt Anthropic expects, joining multiple entries
// with a blank line. It returns "" when there are no system messages.
func toAnthropicSystem(messages []Message) string {
	var parts []string

	for _, m := range messages {
		if m.Role() == SystemRole {
			parts = append(parts, m.(SystemMessage).Content)
		}
	}

	return strings.Join(parts, "\n\n")
}

// toAnthropicMessages converts the conversation into Anthropic messages,
// dropping system messages (carried separately by [toAnthropicSystem]).
// Consecutive messages that map to the same role — most commonly a run of tool
// results answering parallel tool calls — are merged into one message, since
// Anthropic requires user and assistant turns to alternate.
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
			msg := m.(UserMessage)
			block := anthropicContentBlock{Type: "text", Text: msg.Content}
			appendBlocks(string(UserRole), []anthropicContentBlock{block})
		case AssistantRole:
			msg := m.(AssistantMessage)

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
				block := anthropicContentBlock{
					Type:  "tool_use",
					ID:    call.ID,
					Name:  call.Name,
					Input: call.Arguments,
				}
				blocks = append(blocks, block)
			}

			appendBlocks(string(AssistantRole), blocks)
		case ToolRole:
			msg := m.(ToolMessage)
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

	toolList := make([]anthropicTool, 0, len(tools))

	for _, t := range tools {
		anthropicT := anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Schema,
		}
		toolList = append(toolList, anthropicT)
	}

	return toolList
}

func fromAnthropicToAssistantMessage(resp anthropicChatResponse) *AssistantMessage {
	result := AssistantMessage{
		Stats: Stats{
			PromptTokens: resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
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
