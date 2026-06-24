package llm

type RoleName string

const (
	SystemRole    RoleName = "system"
	UserRole      RoleName = "user"
	AssistantRole RoleName = "assistant"
	ToolRole      RoleName = "tool"
)

// Message is a sealed interface: only the message types declared in this
// package satisfy it (via the unexported isMessage marker), so the set of
// chat roles is closed. Role reports the message's role for inspection.
type Message interface {
	Role() RoleName
	isMessage()
}

type SystemMessage struct {
	Content string
}

func (SystemMessage) Role() RoleName {
	return SystemRole
}

func (SystemMessage) isMessage() {}

type UserMessage struct {
	Content string
}

func (UserMessage) Role() RoleName {
	return UserRole
}

func (UserMessage) isMessage() {}

type AssistantMessage struct {
	Content   string
	ToolCalls []ToolCall
	Stats     Stats
}

func (AssistantMessage) Role() RoleName {
	return AssistantRole
}

func (AssistantMessage) isMessage() {}

type ToolMessage struct {
	ToolCallID string
	Content    string
}

func (ToolMessage) Role() RoleName {
	return ToolRole
}

func (ToolMessage) isMessage() {}
