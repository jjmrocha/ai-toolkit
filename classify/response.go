package classify

import "time"

// Answer is the answer to one question. Only the answer types in this package
// implement it, one per [QuestionType]. Type returns which one, so a caller can
// switch on it before reading the fields.
type Answer interface {
	Type() QuestionType
	isAnswer()
}

// YesNoAnswer is the answer to a [YesNo] question.
type YesNoAnswer struct {
	// Value is the probability that the answer is yes, from 0 (no) to 1 (yes).
	Value float64
}

// Type returns [YesNoType].
func (YesNoAnswer) Type() QuestionType {
	return YesNoType
}

func (YesNoAnswer) isAnswer() {}

// ChoiceAnswer is the answer to a [Choice] question.
type ChoiceAnswer struct {
	// Selected is the most probable option.
	Selected string
	// Probabilities maps each option to its probability. They sum to 1.
	Probabilities map[string]float64
	// Confidence is how concentrated Probabilities is, from 0 to 1: how sure the
	// model is, not how likely it is to be right.
	Confidence float64
}

// Type returns [ChoiceType].
func (ChoiceAnswer) Type() QuestionType {
	return ChoiceType
}

func (ChoiceAnswer) isAnswer() {}

// ScoreAnswer is the answer to a [Score] question.
type ScoreAnswer struct {
	// Score is the probability-weighted position across the levels, from 0 to the
	// last level's index. It can fall between two levels.
	Score float64
	// Legend describes each level, in the question's order.
	Legend []string
	// Probabilities is each level's probability, indexed like Legend. They sum to
	// 1.
	Probabilities []float64
	// Confidence is how concentrated Probabilities is, from 0 to 1: how sure the
	// model is, not how likely it is to be right.
	Confidence float64
}

// Type returns [ScoreType].
func (ScoreAnswer) Type() QuestionType {
	return ScoreType
}

func (ScoreAnswer) isAnswer() {}

// Response holds the answers to a [Request].
type Response struct {
	// Model is the model that answered. For an alias, it names the release the
	// alias resolved to.
	Model string
	// Answers maps each identifier from Request.Questions to its answer.
	Answers map[string]Answer
	// Stats is the usage of the request.
	Stats Stats
}

// Stats is the usage of one request.
type Stats struct {
	// InputTokens is the number of tokens in the request. Providers bill these;
	// the answers are free.
	InputTokens int
	// Duration is how long the provider call took, timed by
	// [Classifier.Classify].
	Duration time.Duration
}
