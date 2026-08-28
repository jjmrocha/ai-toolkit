package llm

// Effort controls how much reasoning ("thinking") the model does before it
// answers. Its values are relative rungs, not provider literals: each provider
// maps them onto its own scale, so the same Effort reaches the wire as
// different values on different backends. An empty Effort is treated as
// [EffortOff] by [New] and [LLM.ChangeEffort].
type Effort string

const (
	// EffortOff asks for as little reasoning as the provider allows. It turns
	// reasoning off on OpenRouter and Ollama; Anthropic has no off switch that
	// is safe for tool calling, so it receives the lowest effort level instead.
	EffortOff Effort = "off"
	// EffortLow requests a small amount of reasoning.
	EffortLow Effort = "low"
	// EffortMedium requests a moderate amount of reasoning.
	EffortMedium Effort = "medium"
	// EffortMax requests the most reasoning the provider allows.
	EffortMax Effort = "max"
)

func (e Effort) valid() bool {
	switch e {
	case EffortOff, EffortLow, EffortMedium, EffortMax:
		return true
	default:
		return false
	}
}

var effortLevels = map[Effort]string{
	EffortLow:    "low",
	EffortMedium: "medium",
	EffortMax:    "high",
}

func (e Effort) reasoningLevel() string {
	return effortLevels[e]
}
