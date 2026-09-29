package classify

// QuestionType is the kind of a [Question] and of the [Answer] it gets.
type QuestionType string

const (
	// YesNoType marks a [YesNo] question and a [YesNoAnswer].
	YesNoType QuestionType = "yes_no"
	// ChoiceType marks a [Choice] question and a [ChoiceAnswer].
	ChoiceType QuestionType = "choice"
	// ScoreType marks a [Score] question and a [ScoreAnswer].
	ScoreType QuestionType = "score"
)

// Question is a question to ask about an input. Only the question types in this
// package implement it, so the set is closed. Type returns its kind.
type Question interface {
	Type() QuestionType
	isQuestion()
}

// YesNo is a yes/no question. Its [YesNoAnswer] is the probability of yes.
type YesNo struct {
	// Instructions is the yes/no question.
	Instructions string
	// True describes what a yes means. Optional, but set it together with False:
	// [Classifier.Classify] rejects a question that describes only one answer.
	True string
	// False describes what a no means. Optional, but set together with True.
	False string
}

// Type returns [YesNoType].
func (YesNo) Type() QuestionType {
	return YesNoType
}

func (YesNo) isQuestion() {}

// Choice asks the model to pick one of several options. Its [ChoiceAnswer]
// gives the option picked and each option's probability.
type Choice struct {
	// Instructions is what the model should decide.
	Instructions string
	// Options maps each option to its description, which may be empty.
	Options map[string]string
}

// Type returns [ChoiceType].
func (Choice) Type() QuestionType {
	return ChoiceType
}

func (Choice) isQuestion() {}

// Score asks the model to rate the input on ordered levels. Its [ScoreAnswer]
// gives a probability-weighted position across them.
type Score struct {
	// Instructions is what the model should rate.
	Instructions string
	// Levels describes each level, lowest first.
	Levels []string
}

// Type returns [ScoreType].
func (Score) Type() QuestionType {
	return ScoreType
}

func (Score) isQuestion() {}

// Request is an input and the questions to ask about it. The questions are
// answered in parallel, and none sees another's answer.
type Request struct {
	// Input is the content to judge.
	Input string
	// Questions maps an identifier of the caller's choosing to each question. The
	// identifiers key [Response.Answers] and are not sent to the model.
	Questions map[string]Question
}
