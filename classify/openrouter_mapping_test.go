package classify

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToORQuestions(t *testing.T) {
	t.Run("maps a yes/no question with both criteria", func(t *testing.T) {
		// given
		questions := map[string]Question{
			"is_urgent": YesNo{
				Instructions: "Does this convey urgency?",
				True:         "Explicitly time-sensitive",
				False:        "No urgency expressed",
			},
		}
		expected := map[string]orQuestion{
			"is_urgent": {
				Type:         "noul",
				Instructions: "Does this convey urgency?",
				Criteria:     map[string]string{"true": "Explicitly time-sensitive", "false": "No urgency expressed"},
			},
		}
		// when
		result := toORQuestions(questions)
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("omits the criteria of a yes/no question that describes neither answer", func(t *testing.T) {
		// given
		questions := map[string]Question{"is_urgent": YesNo{Instructions: "Does this convey urgency?"}}
		// when
		result := toORQuestions(questions)
		// then
		assert.Nil(t, result["is_urgent"].Criteria)
	})

	t.Run("keeps the described side of a half-described yes/no question", func(t *testing.T) {
		// given
		questions := map[string]Question{"is_urgent": YesNo{Instructions: "Urgent?", True: "Time-sensitive"}}
		expected := map[string]string{"true": "Time-sensitive"}
		// when
		result := toORQuestions(questions)
		// then
		assert.Equal(t, expected, result["is_urgent"].Criteria)
	})

	t.Run("maps a choice, sending null for an option with no description", func(t *testing.T) {
		// given
		questions := map[string]Question{
			"department": Choice{
				Instructions: "Which team should handle this?",
				Options:      map[string]string{"billing": "Payments and refunds", "sales": ""},
			},
		}
		described := "Payments and refunds"
		expected := map[string]orQuestion{
			"department": {
				Type:         "choice",
				Instructions: "Which team should handle this?",
				Criteria:     map[string]*string{"billing": &described, "sales": nil},
			},
		}
		// when
		result := toORQuestions(questions)
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("maps a score as an ordered list of levels", func(t *testing.T) {
		// given
		questions := map[string]Question{
			"frustration": Score{
				Instructions: "How frustrated is the customer?",
				Levels:       []string{"Calm", "Frustrated", "Very angry"},
			},
		}
		expected := map[string]orQuestion{
			"frustration": {
				Type:         "score",
				Instructions: "How frustrated is the customer?",
				Criteria:     []string{"Calm", "Frustrated", "Very angry"},
			},
		}
		// when
		result := toORQuestions(questions)
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("accepts questions passed as pointers", func(t *testing.T) {
		// given
		questions := map[string]Question{"is_urgent": &YesNo{Instructions: "Urgent?"}}
		// when
		result := toORQuestions(questions)
		// then
		assert.Equal(t, "Urgent?", result["is_urgent"].Instructions)
	})
}

func TestFromORToResponse(t *testing.T) {
	questions := map[string]Question{
		"is_bug":  YesNo{Instructions: "Is this a defect?"},
		"team":    Choice{Instructions: "Which team?", Options: map[string]string{"payments": "", "frontend": ""}},
		"urgency": Score{Instructions: "How urgent?", Levels: []string{"Later", "This week", "Now"}},
	}

	t.Run("maps every answer type and the usage", func(t *testing.T) {
		// given
		apiResp := orDecisionResponse{
			Model: "typesafe/jev-1.13-20260917",
			Answers: map[string]orAnswer{
				"is_bug": {Type: "noul", Noul: 0.96},
				"team": {
					Type:          "choice",
					Choice:        "payments",
					Confidence:    0.67,
					Probabilities: map[string]float64{"payments": 0.78, "frontend": 0.22},
				},
				"urgency": {
					Type:          "score",
					Score:         1.99,
					Confidence:    0.99,
					Probabilities: map[string]float64{"0": 0, "1": 0.01, "2": 0.99},
					Legend:        map[string]string{"0": "Later", "1": "This week", "2": "Now"},
				},
			},
			Usage: orUsage{InputTokens: 476},
		}
		expected := &Response{
			Model: "typesafe/jev-1.13-20260917",
			Answers: map[string]Answer{
				"is_bug": YesNoAnswer{Value: 0.96},
				"team": ChoiceAnswer{
					Selected:      "payments",
					Probabilities: map[string]float64{"payments": 0.78, "frontend": 0.22},
					Confidence:    0.67,
				},
				"urgency": ScoreAnswer{
					Score:         1.99,
					Legend:        []string{"Later", "This week", "Now"},
					Probabilities: []float64{0, 0.01, 0.99},
					Confidence:    0.99,
				},
			},
			Stats: Stats{InputTokens: 476},
		}
		// when
		result, err := fromORToResponse(apiResp, questions)
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	errorCases := []struct {
		name    string
		answers map[string]orAnswer
	}{
		{
			name:    "a question is left unanswered",
			answers: map[string]orAnswer{"is_bug": {Type: "noul", Noul: 0.96}},
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			// given
			apiResp := orDecisionResponse{Answers: tc.answers}
			// when
			result, err := fromORToResponse(apiResp, questions)
			// then
			assert.Nil(t, result)
			assert.ErrorIs(t, err, ErrMissingAnswer)
		})
	}

	malformedCases := []struct {
		name   string
		answer orAnswer
	}{
		{
			name:   "the answer type does not match the question",
			answer: orAnswer{Type: "noul", Noul: 0.5},
		},
		{
			name: "a score level key is not a number",
			answer: orAnswer{
				Type:          "score",
				Probabilities: map[string]float64{"first": 1},
				Legend:        map[string]string{"first": "Later"},
			},
		},
		{
			name: "a score level key is out of range",
			answer: orAnswer{
				Type:          "score",
				Probabilities: map[string]float64{"0": 0.5, "7": 0.5},
				Legend:        map[string]string{"0": "Later", "7": "Now"},
			},
		},
	}

	for _, tc := range malformedCases {
		t.Run(tc.name, func(t *testing.T) {
			// given
			apiResp := orDecisionResponse{Answers: map[string]orAnswer{"urgency": tc.answer}}
			onlyScore := map[string]Question{"urgency": questions["urgency"]}
			// when
			result, err := fromORToResponse(apiResp, onlyScore)
			// then
			assert.Nil(t, result)
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrMissingAnswer)
		})
	}
}
