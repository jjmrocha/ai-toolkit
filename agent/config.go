package agent

import (
	"context"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/skills"
	"github.com/jjmrocha/ai-toolkit/tools"
)

// SessionConfig is what a session gives the model: the system prompt and the
// tools it may call. Pass it to [Agent.StartSession].
type SessionConfig struct {
	// Prompt becomes the system message, and survives [Agent.ResetSession].
	Prompt string
	// ToolBox holds the tools the model may call. Nil means no tools.
	ToolBox *tools.ToolBox
	// Skills are the skills the model may load. Their tools are registered in
	// ToolBox until the session ends, and their catalog is appended to Prompt. Nil
	// or empty means no skills.
	Skills *skills.Collection
	// Messages resumes a saved conversation, such as one from [Agent.Messages].
	// They follow the system message built from Prompt, in order. Any
	// [llm.SystemMessage] among them is skipped, so the new Prompt applies. Nil or
	// empty starts a new conversation.
	Messages []llm.Message
	// ID identifies the session and is used as given, so a resumed conversation
	// keeps its id. Empty means [Agent.StartSession] creates a new one.
	ID string
}

// Interceptor sets the text the model sees for a user message. [Agent.Process]
// calls it once per round, before the first model call, with its own ctx and
// the user's text. The returned text becomes the user message, in the model's
// input and in the conversation. An empty result keeps userInput. It is not
// called when userInput is empty. It runs inside [Agent.Process], so it should
// respect ctx and limit its own time.
type Interceptor func(ctx context.Context, userInput string) string

// Config tunes an [Agent]. The zero value works: no iteration limit, the
// default compaction threshold and no [Interceptor].
type Config struct {
	// MaxIterations caps the model/tool rounds in one [Agent.Process] call, after
	// which it returns [ErrMaxIterations]. Zero means no limit.
	MaxIterations int
	// CompactionThresholdPercent is the share of the context window, in percent,
	// at which [Agent.Process] summarizes the older turns. Zero means 85. Outside
	// 0 to 100, [New] returns [ErrInvalidThreshold].
	CompactionThresholdPercent int
	// Interceptor sets the text the model sees for each user message. Nil means
	// the model sees the user's text as given.
	Interceptor Interceptor
}
