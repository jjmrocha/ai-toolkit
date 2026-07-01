package agent

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
