package llm

import "context"

type llmProvider interface {
	chat(context.Context, []Message, []Tool) (*AssistantMessage, error)
	modelInfo(context.Context) (*ModelInfo, error)
}
