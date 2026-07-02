package llm

import "fmt"

func toOllamaMessages(messages []Message) []ollamaMessage {
	convertedMessages := make([]ollamaMessage, 0, len(messages))

	for _, m := range messages {
		switch m.Role() {
		case SystemRole:
			msg := m.(SystemMessage)
			ollamaMsg := ollamaMessage{
				Role:    string(SystemRole),
				Content: msg.Content,
			}
			convertedMessages = append(convertedMessages, ollamaMsg)
		case UserRole:
			msg := m.(UserMessage)
			ollamaMsg := ollamaMessage{
				Role:    string(UserRole),
				Content: msg.Content,
			}
			convertedMessages = append(convertedMessages, ollamaMsg)
		case AssistantRole:
			msg := m.(AssistantMessage)
			ollamaMsg := ollamaMessage{
				Role:    string(AssistantRole),
				Content: msg.Content,
			}

			for _, call := range msg.ToolCalls {
				toolCall := ollamaToolCall{
					Function: ollamaToolCallFunction{
						Name:      call.Name,
						Arguments: call.Arguments,
					},
				}
				ollamaMsg.ToolCalls = append(ollamaMsg.ToolCalls, toolCall)
			}

			convertedMessages = append(convertedMessages, ollamaMsg)
		case ToolRole:
			msg := m.(ToolMessage)
			ollamaMsg := ollamaMessage{
				Role:     string(ToolRole),
				Content:  msg.Content,
				ToolName: msg.ToolName,
			}
			convertedMessages = append(convertedMessages, ollamaMsg)
		}
	}

	return convertedMessages
}

func toOllamaThink(e Effort) any {
	if e == EffortOff {
		return false
	}

	return e.reasoningLevel()
}

func toOllamaTools(tools []Tool) []ollamaTool {
	if len(tools) == 0 {
		return nil
	}

	toolList := make([]ollamaTool, 0, len(tools))

	for _, t := range tools {
		ollamaT := ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
		}
		toolList = append(toolList, ollamaT)
	}

	return toolList
}

func fromOllamaToAssistantMessage(resp ollamaChatResponse) *AssistantMessage {
	result := AssistantMessage{
		Content: resp.Message.Content,
		Stats: Stats{
			PromptTokens: resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
			TotalTokens:  resp.PromptEvalCount + resp.EvalCount,
		},
	}

	for _, call := range resp.Message.ToolCalls {
		toolCall := ToolCall{
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		}
		result.ToolCalls = append(result.ToolCalls, toolCall)
	}

	return &result
}

func fromOllamaToModelInfo(resp ollamaShowResponse, model string) (*ModelInfo, error) {
	arch, ok := resp.ModelInfo["general.architecture"].(string)
	if !ok {
		return nil, fmt.Errorf("ollama: %w: %q", ErrMissingContextLength, model)
	}

	ctxLen, ok := resp.ModelInfo[arch+".context_length"].(float64)
	if !ok {
		return nil, fmt.Errorf("ollama: %w: %q", ErrMissingContextLength, model)
	}

	return &ModelInfo{Name: model, ContextSize: int(ctxLen)}, nil
}
