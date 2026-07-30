package llm

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

	t.Run("returns zero for a missing or malformed header", func(t *testing.T) {
		// then
		assert.Zero(t, retryAfterWait(""))
		assert.Zero(t, retryAfterWait("later"))
		assert.Zero(t, retryAfterWait("-1"))
	})
}
