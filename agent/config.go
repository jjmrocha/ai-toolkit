package agent

import (
	"github.com/jjmrocha/ai-toolkit/skills"
	"github.com/jjmrocha/ai-toolkit/tools"
)

// SessionConfig declares what a session exposes to the model: the system prompt
// and the tools it may call. Pass it to [Agent.StartSession].
type SessionConfig struct {
	// Prompt becomes the session's system message, preserved across
	// [Agent.ResetSession].
	Prompt string
	// ToolBox holds the tools the model may call during the session. A nil
	// ToolBox is treated as an empty one.
	ToolBox *tools.ToolBox
	// Skills are the skills the model may load during the session. Their tools
	// are registered in ToolBox until the session ends, and their catalog is
	// appended to Prompt. A nil or empty Collection means no skills.
	Skills *skills.Collection
}

// Config tunes an [Agent]'s behavior. The zero value is usable: MaxIterations
// defaults to unbounded and compaction runs at the default threshold.
type Config struct {
	// MaxIterations caps how many model/tool rounds a single [Agent.Process]
	// call may run before it returns [ErrMaxIterations]. Zero means no limit.
	MaxIterations int
	// CompactionThresholdPercent is the percentage of the model's context
	// window at which [Agent.Process] summarizes the older turns. Zero selects
	// the default of 85%. Must be between 0 and 100; otherwise [New] returns
	// [ErrInvalidThreshold].
	CompactionThresholdPercent int
}
