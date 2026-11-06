package decision

import "fmt"

var ollamaQuestionTypes = map[QuestionType]string{
	YesNoType: "noul",
}

func toOllamaQuestions(questions map[string]Question) map[string]ollamaQuestion {
	converted := make(map[string]ollamaQuestion, len(questions))

	for id, q := range questions {
		if q.Type() == YesNoType {
			question := questionValue[YesNo](q)
			converted[id] = ollamaQuestion{
				Type:         ollamaQuestionTypes[YesNoType],
				Instructions: question.Instructions,
				Criteria:     toOllamaYesNoCriteria(question),
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

		if q.Type() == YesNoType {
			answers[id] = YesNoAnswer{Value: apiAnswer.Noul}
		}
	}

	return &Response{
		Answers: answers,
		Stats:   Stats{PromptTokens: resp.Usage.InputTokens},
	}, nil
}
