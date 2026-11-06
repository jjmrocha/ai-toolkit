package decision

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToOllamaQuestions(t *testing.T) {
	t.Run("maps a yes/no question with both criteria", func(t *testing.T) {
		// given
		questions := map[string]Question{
			"refund": YesNo{
				Instructions: "Does the customer ask for a refund?",
				True:         "The customer asks for money back",
				False:        "The customer does not ask for money back",
			},
		}
		expected := map[string]ollamaQuestion{
			"refund": {
				Type:         "noul",
				Instructions: "Does the customer ask for a refund?",
				Criteria:     map[string]string{"true": "The customer asks for money back", "false": "The customer does not ask for money back"},
			},
		}
		// when
		result := toOllamaQuestions(questions)
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("omits the criteria of a yes/no question that describes neither answer", func(t *testing.T) {
		// given
		questions := map[string]Question{"refund": YesNo{Instructions: "Does the customer ask for a refund?"}}
		// when
		result := toOllamaQuestions(questions)
		// then
		assert.Nil(t, result["refund"].Criteria)
	})

	t.Run("maps a choice, sending null for an option with no description", func(t *testing.T) {
		// given
		questions := map[string]Question{
			"team": Choice{
				Instructions: "Which team should handle this ticket?",
				Options:      map[string]string{"billing": "Payments and refunds", "other": ""},
			},
		}
		described := "Payments and refunds"
		expected := map[string]ollamaQuestion{
			"team": {
				Type:         "choice",
				Instructions: "Which team should handle this ticket?",
				Criteria:     map[string]*string{"billing": &described, "other": nil},
			},
		}
		// when
		result := toOllamaQuestions(questions)
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("maps a score as an ordered list of levels", func(t *testing.T) {
		// given
		questions := map[string]Question{
			"urgency": Score{
				Instructions: "How urgent is this ticket?",
				Levels:       []string{"Routine", "Soon", "Urgent"},
			},
		}
		expected := map[string]ollamaQuestion{
			"urgency": {
				Type:         "score",
				Instructions: "How urgent is this ticket?",
				Criteria:     []string{"Routine", "Soon", "Urgent"},
			},
		}
		// when
		result := toOllamaQuestions(questions)
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("accepts questions passed as pointers", func(t *testing.T) {
		// given
		questions := map[string]Question{"refund": &YesNo{Instructions: "Refund?"}}
		// when
		result := toOllamaQuestions(questions)
		// then
		assert.Equal(t, "Refund?", result["refund"].Instructions)
	})
}

func TestFromOllamaToResponse(t *testing.T) {
	questions := map[string]Question{"refund": YesNo{Instructions: "Does the customer ask for a refund?"}}
	choiceAndScore := map[string]Question{
		"team": Choice{
			Instructions: "Which team should handle this ticket?",
			Options:      map[string]string{"billing": "Payments and refunds", "technical": "Bugs and integrations", "other": "None of the above"},
		},
		"urgency": Score{Instructions: "How urgent is this ticket?", Levels: []string{"Routine", "Soon", "Urgent"}},
	}

	t.Run("maps a yes/no answer and the usage", func(t *testing.T) {
		// given
		apiResp := ollamaDecisionResponse{
			Answers: map[string]ollamaAnswer{"refund": {Type: "noul", Noul: 0.997}},
			Usage:   ollamaUsage{InputTokens: 841},
		}
		expected := &Response{
			Answers: map[string]Answer{"refund": YesNoAnswer{Value: 0.997}},
			Stats:   Stats{PromptTokens: 841},
		}
		// when
		result, err := fromOllamaToResponse(apiResp, questions)
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("maps a choice answer and a score answer", func(t *testing.T) {
		// given
		apiResp := ollamaDecisionResponse{
			Answers: map[string]ollamaAnswer{
				"team": {
					Type:          "choice",
					Choice:        "billing",
					Probabilities: map[string]float64{"billing": 0.984, "technical": 0.011, "other": 0.005},
					Confidence:    0.918,
				},
				"urgency": {
					Type:          "score",
					Score:         0.693,
					Probabilities: map[string]float64{"0": 0.482, "1": 0.342, "2": 0.175},
					Confidence:    0.068,
				},
			},
		}
		expected := map[string]Answer{
			"team": ChoiceAnswer{
				Selected:      "billing",
				Probabilities: map[string]float64{"billing": 0.984, "technical": 0.011, "other": 0.005},
				Confidence:    0.918,
			},
			"urgency": ScoreAnswer{
				Score:         0.693,
				Legend:        []string{"Routine", "Soon", "Urgent"},
				Probabilities: []float64{0.482, 0.342, 0.175},
				Confidence:    0.068,
			},
		}
		// when
		result, err := fromOllamaToResponse(apiResp, choiceAndScore)
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, result.Answers)
	})

	t.Run("a score answer that skips a level reports it as zero", func(t *testing.T) {
		// given
		apiResp := ollamaDecisionResponse{
			Answers: map[string]ollamaAnswer{
				"urgency": {Type: "score", Score: 1.2, Probabilities: map[string]float64{"0": 0.4, "2": 0.6}},
			},
		}
		onlyScore := map[string]Question{"urgency": choiceAndScore["urgency"]}
		expected := ScoreAnswer{
			Score:         1.2,
			Legend:        []string{"Routine", "Soon", "Urgent"},
			Probabilities: []float64{0.4, 0, 0.6},
		}
		// when
		result, err := fromOllamaToResponse(apiResp, onlyScore)
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, result.Answers["urgency"])
	})

	badLevelCases := []struct {
		name          string
		probabilities map[string]float64
	}{
		{name: "a score level key is not a number", probabilities: map[string]float64{"first": 1}},
		{name: "a score level key is below zero", probabilities: map[string]float64{"-1": 0.5, "0": 0.5}},
		{name: "a score level key is beyond the last level", probabilities: map[string]float64{"0": 0.5, "3": 0.5}},
	}

	for _, tc := range badLevelCases {
		t.Run(tc.name, func(t *testing.T) {
			// given
			apiResp := ollamaDecisionResponse{
				Answers: map[string]ollamaAnswer{"urgency": {Type: "score", Probabilities: tc.probabilities}},
			}
			onlyScore := map[string]Question{"urgency": choiceAndScore["urgency"]}
			// when
			result, err := fromOllamaToResponse(apiResp, onlyScore)
			// then
			assert.Nil(t, result)
			require.Error(t, err)
			assert.Contains(t, err.Error(), `ollama: question "urgency"`)
		})
	}

	t.Run("a question is left unanswered", func(t *testing.T) {
		// given
		apiResp := ollamaDecisionResponse{Answers: map[string]ollamaAnswer{}}
		// when
		result, err := fromOllamaToResponse(apiResp, questions)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrMissingAnswer)
	})

	t.Run("an answer of another type than its question is an error", func(t *testing.T) {
		// given
		apiResp := ollamaDecisionResponse{Answers: map[string]ollamaAnswer{"refund": {Type: "choice"}}}
		// when
		result, err := fromOllamaToResponse(apiResp, questions)
		// then
		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `answered with type "choice"`)
	})
}
