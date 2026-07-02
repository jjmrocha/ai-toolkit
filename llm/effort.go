package llm

// Effort controls how much reasoning ("thinking") the model does before it
// answers. It maps onto each provider's native reasoning control: an
// extended-thinking token budget for Anthropic and a reasoning-effort level for
// OpenRouter and Ollama. An empty Effort is treated as [EffortOff] by [New].
type Effort string

const (
	// EffortOff disables reasoning; the model answers directly.
	EffortOff Effort = "off"
	// EffortLow requests a small amount of reasoning.
	EffortLow Effort = "low"
	// EffortMedium requests a moderate amount of reasoning.
	EffortMedium Effort = "medium"
	// EffortMax requests the most reasoning the provider allows.
	EffortMax Effort = "max"
)

// valid reports whether e is one of the recognized Effort values.
func (e Effort) valid() bool {
	switch e {
	case EffortOff, EffortLow, EffortMedium, EffortMax:
		return true
	default:
		return false
	}
}

// effortBudgets holds the Anthropic extended-thinking token budget for each
// effort level; it is meaningful only for Anthropic, whose thinking budget must
// be reserved within max_tokens.
var effortBudgets = map[Effort]int{
	EffortOff:    0,
	EffortLow:    2000,
	EffortMedium: 4000,
	EffortMax:    16000,
}

func (e Effort) tokenBudget() int {
	return effortBudgets[e]
}

// effortLevels maps an Effort onto the low/medium/high vocabulary shared by the
// OpenRouter and Ollama reasoning APIs. EffortMax collapses onto "high", the
// strongest level both providers accept ("max" is rejected by OpenRouter and by
// Ollama's top-level think parameter).
var effortLevels = map[Effort]string{
	EffortLow:    "low",
	EffortMedium: "medium",
	EffortMax:    "high",
}

func (e Effort) reasoningLevel() string {
	return effortLevels[e]
}
