package classify

import (
	"fmt"
	"strconv"
)

type orDecisionRequest struct {
	Model     string                `json:"model"`
	State     string                `json:"state"`
	Questions map[string]orQuestion `json:"questions"`
}

type orQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type orDecisionResponse struct {
	Model   string              `json:"model"`
	Answers map[string]orAnswer `json:"answers"`
	Usage   orUsage             `json:"usage"`
	Error   *orDecisionError    `json:"error"`
}

type orAnswer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul"`
	Choice        string             `json:"choice"`
	Score         float64            `json:"score"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
	Legend        map[string]string  `json:"legend"`
}

type orUsage struct {
	InputTokens int `json:"input_tokens"`
}

type orDecisionError struct {
	Message string `json:"message"`
}

var orQuestionTypes = map[QuestionType]string{
	YesNoType:  "noul",
	ChoiceType: "choice",
	ScoreType:  "score",
}

func questionValue[T Question](q Question) T {
	if v, ok := q.(T); ok {
		return v
	}

	return *(any(q).(*T))
}

func toORQuestions(questions map[string]Question) map[string]orQuestion {
	converted := make(map[string]orQuestion, len(questions))

	for id, q := range questions {
		switch q.Type() {
		case YesNoType:
			question := questionValue[YesNo](q)
			converted[id] = orQuestion{
				Type:         orQuestionTypes[YesNoType],
				Instructions: question.Instructions,
				Criteria:     toORYesNoCriteria(question),
			}
		case ChoiceType:
			question := questionValue[Choice](q)
			converted[id] = orQuestion{
				Type:         orQuestionTypes[ChoiceType],
				Instructions: question.Instructions,
				Criteria:     toORChoiceCriteria(question),
			}
		case ScoreType:
			question := questionValue[Score](q)
			converted[id] = orQuestion{
				Type:         orQuestionTypes[ScoreType],
				Instructions: question.Instructions,
				Criteria:     question.Levels,
			}
		}
	}

	return converted
}

func toORYesNoCriteria(question YesNo) any {
	criteria := make(map[string]string, 2)

	if question.True != "" {
		criteria["true"] = question.True
	}

	if question.False != "" {
		criteria["false"] = question.False
	}

	if len(criteria) == 0 {
		return nil
	}

	return criteria
}

func toORChoiceCriteria(question Choice) map[string]*string {
	criteria := make(map[string]*string, len(question.Options))

	for option, description := range question.Options {
		if description == "" {
			criteria[option] = nil
			continue
		}

		criteria[option] = &description
	}

	return criteria
}

func fromORToResponse(resp orDecisionResponse, questions map[string]Question) (*Response, error) {
	answers := make(map[string]Answer, len(questions))

	for id, q := range questions {
		apiAnswer, ok := resp.Answers[id]
		if !ok {
			return nil, fmt.Errorf("openrouter: %w for question %q", ErrMissingAnswer, id)
		}

		if apiAnswer.Type != orQuestionTypes[q.Type()] {
			return nil, fmt.Errorf("openrouter: question %q of type %q answered with type %q", id, q.Type(), apiAnswer.Type)
		}

		switch q.Type() {
		case YesNoType:
			answers[id] = YesNoAnswer{Value: apiAnswer.Noul}
		case ChoiceType:
			answers[id] = ChoiceAnswer{
				Selected:      apiAnswer.Choice,
				Probabilities: apiAnswer.Probabilities,
				Confidence:    apiAnswer.Confidence,
			}
		case ScoreType:
			answer, err := fromORToScoreAnswer(apiAnswer)
			if err != nil {
				return nil, fmt.Errorf("openrouter: question %q: %w", id, err)
			}

			answers[id] = answer
		}
	}

	return &Response{
		Model:   resp.Model,
		Answers: answers,
		Stats:   Stats{InputTokens: resp.Usage.InputTokens},
	}, nil
}

func fromORToScoreAnswer(apiAnswer orAnswer) (ScoreAnswer, error) {
	legend := make([]string, len(apiAnswer.Legend))

	for key, description := range apiAnswer.Legend {
		level, err := toLevel(key, len(legend))
		if err != nil {
			return ScoreAnswer{}, err
		}

		legend[level] = description
	}

	probabilities := make([]float64, len(apiAnswer.Probabilities))

	for key, probability := range apiAnswer.Probabilities {
		level, err := toLevel(key, len(probabilities))
		if err != nil {
			return ScoreAnswer{}, err
		}

		probabilities[level] = probability
	}

	return ScoreAnswer{
		Score:         apiAnswer.Score,
		Legend:        legend,
		Probabilities: probabilities,
		Confidence:    apiAnswer.Confidence,
	}, nil
}

func toLevel(key string, levels int) (int, error) {
	level, err := strconv.Atoi(key)
	if err != nil {
		return 0, fmt.Errorf("level %q is not a number", key)
	}

	if level < 0 || level >= levels {
		return 0, fmt.Errorf("level %d is outside the %d levels answered", level, levels)
	}

	return level, nil
}
