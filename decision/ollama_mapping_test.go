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
