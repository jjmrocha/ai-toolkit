package agent

import (
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

// Config tunes an [Agent]. The zero value works: no iteration limit and the
// default compaction threshold.
type Config struct {
	// MaxIterations caps the model/tool rounds in one [Agent.Process] call, after
	// which it returns [ErrMaxIterations]. Zero means no limit.
	MaxIterations int
	// CompactionThresholdPercent is the share of the context window, in percent,
	// at which [Agent.Process] summarizes the older turns. Zero means 85. Outside
	// 0 to 100, [New] returns [ErrInvalidThreshold].
	CompactionThresholdPercent int
}
