package packs

import (
	"testing"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/fn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedNow(t *testing.T, value time.Time) {
	t.Helper()

	original := now
	now = func() time.Time { return value }

	t.Cleanup(func() { now = original })
}

func runDateTool(t *testing.T, name string) string {
	t.Helper()

	toolBox := tools.NewToolBox()

	pack, err := DateTools(toolBox)
	require.NoError(t, err)

	defer func() { _ = pack.Close() }()

	call := llm.ToolCall{ID: "call-1", Name: name}

	message, err := toolBox.Execute(t.Context(), call)
	require.NoError(t, err)

	return message.Content
}

func TestDateTools(t *testing.T) {
	t.Run("registers the three tools", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()
		// when
		pack, err := DateTools(toolBox)
		require.NoError(t, err)

		defer func() { _ = pack.Close() }()
		// then
		expected := []string{currentDateToolName, currentTimeToolName, timeZoneToolName}
		result := fn.Map(toolBox.Tools(), func(tool llm.Tool) string { return tool.Name })
		assert.ElementsMatch(t, expected, result)
	})

	t.Run("removes the three tools on close", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()

		pack, err := DateTools(toolBox)
		require.NoError(t, err)
		// when
		require.NoError(t, pack.Close())
		// then
		assert.Empty(t, toolBox.Tools())
	})

	t.Run("reports the date as year, month and day", func(t *testing.T) {
		// given
		fixedNow(t, time.Date(2026, time.September, 20, 15, 4, 5, 0, time.UTC))
		// when
		result := runDateTool(t, currentDateToolName)
		// then
		assert.Equal(t, "2026-09-20", result)
	})

	t.Run("reports the time to the millisecond on a 24 hour clock", func(t *testing.T) {
		testCases := []struct {
			name     string
			value    time.Time
			expected string
		}{
			{
				name:     "afternoon",
				value:    time.Date(2026, time.September, 20, 15, 4, 5, 250_000_000, time.UTC),
				expected: "15:04:05.250",
			},
			{
				name:     "single digits padded",
				value:    time.Date(2026, time.September, 20, 9, 5, 3, 7_000_000, time.UTC),
				expected: "09:05:03.007",
			},
			{
				name:     "midnight",
				value:    time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC),
				expected: "00:00:00.000",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				// given
				fixedNow(t, testCase.value)
				// when
				result := runDateTool(t, currentTimeToolName)
				// then
				assert.Equal(t, testCase.expected, result)
			})
		}
	})

	t.Run("reports the host zone with its offset from utc", func(t *testing.T) {
		testCases := []struct {
			name     string
			zone     *time.Location
			expected string
		}{
			{
				name:     "named ahead of utc",
				zone:     time.FixedZone("WEST", 3600),
				expected: "WEST (UTC+01:00)",
			},
			{
				name:     "named behind utc",
				zone:     time.FixedZone("EST", -5*3600),
				expected: "EST (UTC-05:00)",
			},
			{
				name:     "utc itself",
				zone:     time.UTC,
				expected: "UTC (UTC+00:00)",
			},
			{
				name:     "half hour offset",
				zone:     time.FixedZone("IST", 5*3600+1800),
				expected: "IST (UTC+05:30)",
			},
			{
				name:     "negative half hour offset",
				zone:     time.FixedZone("NST", -(3*3600 + 1800)),
				expected: "NST (UTC-03:30)",
			},
			{
				name:     "unnamed zone",
				zone:     time.FixedZone("", 45*60),
				expected: "UTC+00:45",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				// given
				fixedNow(t, time.Date(2026, time.September, 20, 15, 4, 5, 0, testCase.zone))
				// when
				result := runDateTool(t, timeZoneToolName)
				// then
				assert.Equal(t, testCase.expected, result)
			})
		}
	})
}
