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

		result = append(result, anthropicMessage{Role: role, Content: blocks})
	}

	for _, m := range messages {
		switch m.Role() {
		case UserRole:
			msg := m.(UserMessage)
			appendBlocks(string(UserRole), []anthropicContentBlock{{Type: "text", Text: msg.Content}})
		case AssistantRole:
			msg := m.(AssistantMessage)

			var blocks []anthropicContentBlock
			if msg.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: msg.Content})
			}

			for _, call := range msg.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    call.ID,
					Name:  call.Name,
					Input: call.Arguments,
				})
			}

			appendBlocks(string(AssistantRole), blocks)
		case ToolRole:
			msg := m.(ToolMessage)
			appendBlocks(string(UserRole), []anthropicContentBlock{{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			}})
		}
	}

	return result
}

func toAnthropicTools(tools []Tool) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}

	toolList := make([]anthropicTool, 0, len(tools))

	for _, t := range tools {
		toolList = append(toolList, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Schema,
		})
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
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	return &result
}

func fromAnthropicToModelInfo(model anthropicModel) (*ModelInfo, error) {
	if model.MaxInputTokens == 0 {
		return nil, fmt.Errorf("anthropic: %w: %q", ErrMissingContextLength, model.ID)
	}

	return &ModelInfo{
		Name:        model.DisplayName,
		ContextSize: model.MaxInputTokens,
	}, nil
}
