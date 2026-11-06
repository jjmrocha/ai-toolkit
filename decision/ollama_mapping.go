package decision

import (
	"fmt"
	"slices"
)

var ollamaQuestionTypes = map[QuestionType]string{
	YesNoType:  "noul",
	ChoiceType: "choice",
	ScoreType:  "score",
}

func toOllamaQuestions(questions map[string]Question) map[string]ollamaQuestion {
	converted := make(map[string]ollamaQuestion, len(questions))

	for id, q := range questions {
		switch q.Type() {
		case YesNoType:
			question := questionValue[YesNo](q)
			converted[id] = ollamaQuestion{
				Type:         ollamaQuestionTypes[YesNoType],
				Instructions: question.Instructions,
				Criteria:     toOllamaYesNoCriteria(question),
			}
		case ChoiceType:
			question := questionValue[Choice](q)
			converted[id] = ollamaQuestion{
				Type:         ollamaQuestionTypes[ChoiceType],
				Instructions: question.Instructions,
				Criteria:     toOllamaChoiceCriteria(question),
			}
		case ScoreType:
			question := questionValue[Score](q)
			converted[id] = ollamaQuestion{
				Type:         ollamaQuestionTypes[ScoreType],
				Instructions: question.Instructions,
				Criteria:     question.Levels,
			}
		}
	}

	return converted
}

func toOllamaYesNoCriteria(question YesNo) any {
	if question.True == "" && question.False == "" {
		return nil
	}

	return map[string]string{"true": question.True, "false": question.False}
}

func toOllamaChoiceCriteria(question Choice) map[string]*string {
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

func fromOllamaToResponse(resp ollamaDecisionResponse, questions map[string]Question) (*Response, error) {
	answers := make(map[string]Answer, len(questions))

	for id, q := range questions {
		apiAnswer, ok := resp.Answers[id]
		if !ok {
			return nil, fmt.Errorf("ollama: %w for question %q", ErrMissingAnswer, id)
		}

		if apiAnswer.Type != ollamaQuestionTypes[q.Type()] {
			return nil, fmt.Errorf("ollama: question %q of type %q answered with type %q", id, q.Type(), apiAnswer.Type)
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
			question := questionValue[Score](q)

			answer, err := fromOllamaToScoreAnswer(apiAnswer, question)
			if err != nil {
				return nil, fmt.Errorf("ollama: question %q: %w", id, err)
			}

			answers[id] = answer
		}
	}

	return &Response{
		Answers: answers,
		Stats:   Stats{PromptTokens: resp.Usage.InputTokens},
	}, nil
}

func fromOllamaToScoreAnswer(apiAnswer ollamaAnswer, question Score) (ScoreAnswer, error) {
	probabilities := make([]float64, len(question.Levels))

	for key, probability := range apiAnswer.Probabilities {
		level, err := toLevel(key, len(probabilities))
		if err != nil {
			return ScoreAnswer{}, err
		}

		probabilities[level] = probability
	}

	return ScoreAnswer{
		Score:         apiAnswer.Score,
		Legend:        slices.Clone(question.Levels),
		Probabilities: probabilities,
		Confidence:    apiAnswer.Confidence,
	}, nil
}
