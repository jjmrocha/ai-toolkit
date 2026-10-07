package rest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryAfterWait(t *testing.T) {
	t.Run("parses a seconds value", func(t *testing.T) {
		// when
		result := retryAfterWait("3")
		// then
		assert.Equal(t, 3*time.Second, result)
	})

	t.Run("caps the wait at the retry maximum", func(t *testing.T) {
		// when
		result := retryAfterWait("300")
		// then
		assert.Equal(t, retryMaxWaitTime, result)
	})

	testCases := []struct {
		name  string
		input string
	}{
		{name: "returns zero for a missing header", input: ""},
		{name: "returns zero for a header that is not a number", input: "later"},
		{name: "returns zero for a negative number of seconds", input: "-1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// given
			input := tc.input
			// when
			result := retryAfterWait(input)
			// then
			assert.Zero(t, result)
		})
	}
}
