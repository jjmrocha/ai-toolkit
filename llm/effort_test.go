package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEffortTokenBudget(t *testing.T) {
	tests := []struct {
		name     string
		input    Effort
		expected int
	}{
		{name: "off has no budget", input: EffortOff, expected: 0},
		{name: "low budget", input: EffortLow, expected: 2000},
		{name: "medium budget", input: EffortMedium, expected: 4000},
		{name: "max budget", input: EffortMax, expected: 16000},
		{name: "unknown effort has no budget", input: Effort("bogus"), expected: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// when
			result := tc.input.tokenBudget()
			// then
			assert.Equal(t, tc.expected, result)
		})
	}
}
