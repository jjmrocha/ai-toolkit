package classify

import "time"

// Answer is a sealed interface: only the answer types declared in this package
// satisfy it, one per [QuestionType]. Type reports which one, so a caller can
// switch on it before reading the answer's fields.
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
	// Selected is the option with the highest probability.
	Selected string
	// Probabilities maps each option offered to its probability. The values sum
	// to 1.
	Probabilities map[string]float64
	// Confidence is how concentrated Probabilities is, from 0 to 1. It reports
	// how certain the model is, not how likely the answer is to be right.
	Confidence float64
}

// Type returns [ChoiceType].
func (ChoiceAnswer) Type() QuestionType {
	return ChoiceType
}

func (ChoiceAnswer) isAnswer() {}

// ScoreAnswer is the answer to a [Score] question.
type ScoreAnswer struct {
	// Score is the probability-weighted position across the levels, from 0 to
	// the index of the last level. It can fall between two levels.
	Score float64
	// Legend describes each level, in the order the question listed them.
	Legend []string
	// Probabilities holds the probability of each level, indexed as Legend is.
	// The values sum to 1.
	Probabilities []float64
	// Confidence is how concentrated Probabilities is, from 0 to 1. It reports
	// how certain the model is, not how likely the answer is to be right.
	Confidence float64
}

// Type returns [ScoreType].
func (ScoreAnswer) Type() QuestionType {
	return ScoreType
}

func (ScoreAnswer) isAnswer() {}

// Response holds the answers to a [Request].
type Response struct {
	// Model is the identifier of the model that answered, which for an alias
	// names the release the alias resolved to.
	Model string
	// Answers maps each identifier from Request.Questions to its answer.
	Answers map[string]Answer
	// Stats reports the usage of the request that produced these answers.
	Stats Stats
}

// Stats reports the usage of a request.
type Stats struct {
	// InputTokens is the number of tokens in the request. Providers bill these
	// and leave the answers free.
	InputTokens int
	// Duration is how long the request took, measured by [Classifier.Classify] around
	// the call to the provider.
	Duration time.Duration
}
