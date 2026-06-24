package llm

// RoleName identifies the role of a chat [Message].
type RoleName string

const (
	// SystemRole marks a [SystemMessage].
	SystemRole RoleName = "system"
	// UserRole marks a [UserMessage].
	UserRole RoleName = "user"
	// AssistantRole marks an [AssistantMessage].
	AssistantRole RoleName = "assistant"
	// ToolRole marks a [ToolMessage].
	ToolRole RoleName = "tool"
)

// Message is a sealed interface: only the message types declared in this
// package satisfy it (via the unexported isMessage marker), so the set of
// chat roles is closed. Role reports the message's role for inspection.
type Message interface {
	Role() RoleName
	isMessage()
}

// SystemMessage carries instructions that steer the model's behavior.
type SystemMessage struct {
	Content string
}

// Role returns [SystemRole].
func (SystemMessage) Role() RoleName {
	return SystemRole
}

func (SystemMessage) isMessage() {}

// UserMessage carries input from the end user.
type UserMessage struct {
	Content string
}

// Role returns [UserRole].
func (UserMessage) Role() RoleName {
	return UserRole
}

func (UserMessage) isMessage() {}

// AssistantMessage is a reply from the model. It may request tool calls, and
// carries token-usage [Stats] when returned by [LLM.Chat].
type AssistantMessage struct {
	Content   string
	ToolCalls []ToolCall
	Stats     Stats
}

// Role returns [AssistantRole].
func (AssistantMessage) Role() RoleName {
	return AssistantRole
}

func (AssistantMessage) isMessage() {}

// ToolMessage carries the result of a [ToolCall] back to the model. ToolCallID
// must match the ID of the originating call.
type ToolMessage struct {
	ToolCallID string
	Content    string
}

// Role returns [ToolRole].
func (ToolMessage) Role() RoleName {
	return ToolRole
}

func (ToolMessage) isMessage() {}
