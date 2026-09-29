package llm

// Effort sets how much the model reasons ("thinks") before it answers. The
// values are relative levels: each provider maps them onto its own scale, so
// the same Effort is sent as different values to different providers. [New]
// and [LLM.ChangeEffort] treat an empty Effort as [EffortOff].
type Effort string

const (
	// EffortOff asks for as little reasoning as the provider allows. OpenRouter and
	// Ollama turn reasoning off. Anthropic has no off switch that is safe with tool
	// calls, so it gets its lowest level.
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
