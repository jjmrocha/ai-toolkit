package llm

import (
	"encoding/json"
	"fmt"
)

func toORMessages(messages []Message) ([]orMessage, error) {
	convertedMessages := make([]orMessage, 0, len(messages))

	for _, m := range messages {
		switch m.Role() {
		case SystemRole:
			msg := m.(SystemMessage)
			orMsg := orMessage{
				Role:    string(SystemRole),
				Content: msg.Content,
			}
			convertedMessages = append(convertedMessages, orMsg)
		case UserRole:
			msg := m.(UserMessage)
			orMsg := orMessage{
				Role:    string(UserRole),
				Content: msg.Content,
			}
			convertedMessages = append(convertedMessages, orMsg)
		case AssistantRole:
			msg := m.(AssistantMessage)
			orMsg := orMessage{
				Role:    string(AssistantRole),
				Content: msg.Content,
			}

			for _, call := range msg.ToolCalls {
				args, err := json.Marshal(call.Arguments)
				if err != nil {
					return nil, fmt.Errorf("openrouter: encoding tool call arguments: %w", err)
				}

				toolCall := orToolCall{
					ID:   call.ID,
					Type: "function",
					Function: orToolCallFunction{
						Name:      call.Name,
						Arguments: string(args),
					},
				}
				orMsg.ToolCalls = append(orMsg.ToolCalls, toolCall)
			}

			convertedMessages = append(convertedMessages, orMsg)
		case ToolRole:
			msg := m.(ToolMessage)
			orMsg := orMessage{
				Role:       string(ToolRole),
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			}
			convertedMessages = append(convertedMessages, orMsg)
		}
	}

	return convertedMessages, nil
}

func toORReasoning(e Effort) *orReasoning {
	if e == EffortOff {
		disabled := false
		return &orReasoning{Enabled: &disabled}
	}

	return &orReasoning{Effort: e.reasoningLevel()}
}

func toORTools(tools []Tool) []orTool {
	if len(tools) == 0 {
		return nil
	}

	toolList := make([]orTool, 0, len(tools))

	for _, t := range tools {
		orT := orTool{
			Type: "function",
			Function: orToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
		}
		toolList = append(toolList, orT)
	}

	return toolList
}

func fromORToAssistantMessage(resp orChatResponse) (*AssistantMessage, error) {
	choice := resp.Choices[0].Message

	result := AssistantMessage{
		Content: choice.Content,
		Stats: Stats{
			PromptTokens: resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}

	for _, call := range choice.ToolCalls {
		var args map[string]any

		if call.Function.Arguments != "" {
			byteArgs := []byte(call.Function.Arguments)
			if err := json.Unmarshal(byteArgs, &args); err != nil {
				return nil, fmt.Errorf("openrouter: decoding tool call arguments: %w", err)
			}
		}

		call := ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: args,
		}
		result.ToolCalls = append(result.ToolCalls, call)
	}

	return &result, nil
}

func fromORToModelInfo(models []orModel, id string) (*ModelInfo, error) {
	for _, m := range models {
		if m.ID == id {
			if m.ContextLength == 0 {
				return nil, fmt.Errorf("openrouter: %w: %q", ErrMissingContextLength, id)
			}

			return &ModelInfo{
				Name:        id,
				ContextSize: m.ContextLength,
			}, nil
		}
	}

	return nil, fmt.Errorf("openrouter: %w: %q", ErrModelNotFound, id)
}
