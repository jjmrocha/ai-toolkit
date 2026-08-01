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

// messageValue returns the concrete message of type T carried by m, accepting
// both the value and pointer forms — callers may seed history with either.
func messageValue[T Message](m Message) T {
	if v, ok := m.(T); ok {
		return v
	}

	return *(any(m).(*T))
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

	// StopReason is the provider's native reason the model stopped generating
	// (e.g. Anthropic's "end_turn" or "max_tokens", OpenRouter's "stop" or
	// "length", Ollama's "stop"). Empty for messages not produced by [LLM.Chat].
	StopReason string

	// raw holds the provider's original response payload so it can be replayed
	// verbatim on the next turn. Anthropic needs this to preserve thinking
	// blocks and their signatures, which Content and ToolCalls don't capture.
	// Empty for messages not produced by a provider (e.g. seeded history), in
	// which case the mapper rebuilds the payload from Content and ToolCalls.
	raw any
}

// Role returns [AssistantRole].
func (AssistantMessage) Role() RoleName {
	return AssistantRole
}

func (AssistantMessage) isMessage() {}

// ToolMessage carries the result of a [ToolCall] back to the model. Providers
// correlate the result either by call ID (e.g. OpenRouter) or by tool name
// (e.g. Ollama), so both fields are available; set whichever the provider uses.
type ToolMessage struct {
	// ToolCallID matches the ID of the originating [ToolCall].
	ToolCallID string
	// ToolName is the name of the tool that produced the result.
	ToolName string
	// Content is the tool's result.
	Content string
}

// Role returns [ToolRole].
func (ToolMessage) Role() RoleName {
	return ToolRole
}

func (ToolMessage) isMessage() {}
