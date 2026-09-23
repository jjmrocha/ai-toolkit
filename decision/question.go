package decision

// QuestionType identifies the kind of a [Question] and of the [Answer] it
// produces.
type QuestionType string

const (
	// NoulType marks a [Noul] question and a [NoulAnswer].
	NoulType QuestionType = "noul"
	// ChoiceType marks a [Choice] question and a [ChoiceAnswer].
	ChoiceType QuestionType = "choice"
	// ScoreType marks a [Score] question and a [ScoreAnswer].
	ScoreType QuestionType = "score"
)

// Question is a sealed interface: only the question types declared in this
// package satisfy it, so the set of questions a model can be asked is closed.
// Type reports the question's kind for inspection.
type Question interface {
	Type() QuestionType
	isQuestion()
}

// Noul is a yes/no question. Its [NoulAnswer] reports the probability that the
// answer is yes.
type Noul struct {
	// Instructions is the yes/no question to evaluate.
	Instructions string
	// True describes what a yes means. Optional.
	True string
	// False describes what a no means. Optional.
	False string
}

// Type returns [NoulType].
func (Noul) Type() QuestionType {
	return NoulType
}

func (Noul) isQuestion() {}

// Choice asks the model to select one of a set of options. Its [ChoiceAnswer]
// reports the selected option and a probability for each one.
type Choice struct {
	// Instructions is what the model should decide.
	Instructions string
	// Options maps each option to a description of it. A description may be
	// empty when the option needs no extra detail.
	Options map[string]string
}

// Type returns [ChoiceType].
func (Choice) Type() QuestionType {
	return ChoiceType
}

func (Choice) isQuestion() {}

// Score asks the model to rate the state against ordered levels. Its
// [ScoreAnswer] reports a probability-weighted position across them.
type Score struct {
	// Instructions is what the model should rate.
	Instructions string
	// Levels describes each level, from lowest to highest.
	Levels []string
}

// Type returns [ScoreType].
func (Score) Type() QuestionType {
	return ScoreType
}

func (Score) isQuestion() {}

// Request is a state to evaluate and the questions to ask about it. The
// questions are evaluated in parallel and in isolation: none of them sees
// another's answer.
type Request struct {
	// State is the content to evaluate.
	State string
	// Questions maps an identifier the caller chooses to the question to ask.
	// The identifiers key [Response.Answers]; they are not sent to the model.
	Questions map[string]Question
}
