package packs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jjmrocha/ai-toolkit/decision"
	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	decisionYesNoToolName  = "decision_yes_no"
	decisionChoiceToolName = "decision_choice"
	decisionScoreToolName  = "decision_score"
	stateArg               = "state"
	instructionsArg        = "instructions"
	trueArg                = "true"
	falseArg               = "false"
	optionsArg             = "options"
	optionNameArg          = "name"
	optionDescriptionArg   = "description"
	levelsArg              = "levels"
	decisionQuestionID     = "question"
)

var decisionToolNames = []string{
	decisionYesNoToolName,
	decisionChoiceToolName,
	decisionScoreToolName,
}

type decisionPack struct {
	toolBox *tools.ToolBox
	once    sync.Once
}

func (p *decisionPack) Close() error {
	p.once.Do(func() {
		for _, name := range decisionToolNames {
			p.toolBox.Remove(name)
		}
	})

	return nil
}

type yesNoResult struct {
	YesProbability float64 `json:"yes_probability"`
}

type choiceResult struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

type scoreResult struct {
	Score         float64            `json:"score"`
	Probabilities []levelProbability `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

type levelProbability struct {
	Level       string  `json:"level"`
	Probability float64 `json:"probability"`
}

// DecisionTools registers the three tools that let the model hand a judgement
// call to the decision model behind client, one question per call:
// "decision_yes_no" asks a [decision.Noul], "decision_choice" a
// [decision.Choice] and "decision_score" a [decision.Score]. Each takes the
// state to judge and the question's instructions, and returns the answer as
// JSON — the probability of yes, the option picked with the probability of each,
// or the position across the levels with the probability of each. The returned
// [ToolPack] removes the three again.
//
// Whatever the model puts in the state is sent to the decision provider, and
// every call is billed on its input tokens, the whole state included.
//
// A choice with fewer than two options or the same option twice, and a score
// with fewer than two levels, fail with [ErrInvalidQuestion] before the provider
// is called. An error from [decision.Decision.Ask] is returned as it is.
//
// The caller owns client: [ToolPack.Close] only unregisters the tools, and a
// dropped pack costs nothing beyond the tools staying registered.
//
// It fails with the error [tools.ToolBox.Add] returns when m rejects a
// registration, which leaves any tool already registered by the call in place.
func DecisionTools(m *tools.ToolBox, client *decision.Decision) (ToolPack, error) {
	yesNoTool := llm.Tool{
		Name: decisionYesNoToolName,
		Description: "Ask a calibrated decision model a yes/no question about a state, instead of " +
			"judging it yourself. Returns {\"yes_probability\": p}, the probability that the answer is " +
			"yes: 0.5 means undecided, not a middle answer. The model sees only the state and the question, " +
			"so make the state self-contained.",
		Schema: tools.NewObjectBuilder().
			String(stateArg, "the content to judge", true).
			String(instructionsArg, "the yes/no question to answer about the state", true).
			String(trueArg, "what a yes means", false).
			String(falseArg, "what a no means", false).
			Build(),
	}

	err := m.Add(yesNoTool, func(ctx context.Context, args map[string]any) (string, error) {
		return askYesNo(ctx, client, args)
	})
	if err != nil {
		return nil, err
	}

	choiceTool := llm.Tool{
		Name: decisionChoiceToolName,
		Description: "Ask a calibrated decision model to pick one of several options for a state, " +
			"instead of choosing yourself. Returns {\"choice\", \"probabilities\", \"confidence\"}: the " +
			"most probable option, the probability of each, and how concentrated they are from 0 to 1 — " +
			"how sure the model is, not how likely it is to be right. The model sees only the state and " +
			"the question, so make the state self-contained.",
		Schema: tools.NewObjectBuilder().
			String(stateArg, "the content to judge", true).
			String(instructionsArg, "what to decide about the state", true).
			ArrayOfObjects(optionsArg, "the options to pick from", true, tools.NewObjectBuilder().
				String(optionNameArg, "the option's name, returned as the choice", true).
				String(optionDescriptionArg, "what the option covers", false)).
			Build(),
	}

	err = m.Add(choiceTool, func(ctx context.Context, args map[string]any) (string, error) {
		return askChoice(ctx, client, args)
	})
	if err != nil {
		return nil, err
	}

	scoreTool := llm.Tool{
		Name: decisionScoreToolName,
		Description: "Ask a calibrated decision model to rate a state against ordered levels, instead of " +
			"rating it yourself. Returns {\"score\", \"probabilities\", \"confidence\"}: a " +
			"probability-weighted position from 0 to the last level's index, which can fall between two " +
			"levels; the probability of each level; and how concentrated they are from 0 to 1. The model " +
			"sees only the state and the question, so make the state self-contained.",
		Schema: tools.NewObjectBuilder().
			String(stateArg, "the content to judge", true).
			String(instructionsArg, "what to rate about the state", true).
			ArrayOfStrings(levelsArg, "a description of each level, from lowest to highest", true).
			Build(),
	}

	err = m.Add(scoreTool, func(ctx context.Context, args map[string]any) (string, error) {
		return askScore(ctx, client, args)
	})
	if err != nil {
		return nil, err
	}

	return &decisionPack{toolBox: m}, nil
}

func askYesNo(ctx context.Context, client *decision.Decision, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	state, instructions, err := stateAndInstructions(arguments)
	if err != nil {
		return "", err
	}

	question := decision.Noul{Instructions: instructions}

	if question.True, err = optionalString(arguments, trueArg); err != nil {
		return "", err
	}

	if question.False, err = optionalString(arguments, falseArg); err != nil {
		return "", err
	}

	answer, err := askOne[decision.NoulAnswer](ctx, client, state, question)
	if err != nil {
		return "", err
	}

	return toJSON(yesNoResult{YesProbability: answer.Value})
}

func askChoice(ctx context.Context, client *decision.Decision, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	state, instructions, err := stateAndInstructions(arguments)
	if err != nil {
		return "", err
	}

	options, err := arguments.GetArrayOfObjects(optionsArg)
	if err != nil {
		return "", err
	}

	question := decision.Choice{Instructions: instructions, Options: make(map[string]string, len(options))}

	for _, option := range options {
		var name, description string

		if name, err = option.GetString(optionNameArg); err != nil {
			return "", err
		}

		if description, err = optionalString(option, optionDescriptionArg); err != nil {
			return "", err
		}

		if _, ok := question.Options[name]; ok {
			return "", fmt.Errorf("%w: option %q is given more than once", ErrInvalidQuestion, name)
		}

		question.Options[name] = description
	}

	if len(question.Options) < 2 {
		return "", fmt.Errorf("%w: %d options given, expected 2 or more", ErrInvalidQuestion, len(question.Options))
	}

	answer, err := askOne[decision.ChoiceAnswer](ctx, client, state, question)
	if err != nil {
		return "", err
	}

	return toJSON(choiceResult{
		Choice:        answer.Choice,
		Probabilities: answer.Probabilities,
		Confidence:    answer.Confidence,
	})
}

func askScore(ctx context.Context, client *decision.Decision, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	state, instructions, err := stateAndInstructions(arguments)
	if err != nil {
		return "", err
	}

	levels, err := arguments.GetArrayOfStrings(levelsArg)
	if err != nil {
		return "", err
	}

	if len(levels) < 2 {
		return "", fmt.Errorf("%w: %d levels given, expected 2 or more", ErrInvalidQuestion, len(levels))
	}

	question := decision.Score{Instructions: instructions, Levels: levels}

	answer, err := askOne[decision.ScoreAnswer](ctx, client, state, question)
	if err != nil {
		return "", err
	}

	levelCount := min(len(answer.Legend), len(answer.Probabilities))
	probabilities := make([]levelProbability, 0, levelCount)

	for i := range levelCount {
		probability := levelProbability{Level: answer.Legend[i], Probability: answer.Probabilities[i]}
		probabilities = append(probabilities, probability)
	}

	return toJSON(scoreResult{
		Score:         answer.Score,
		Probabilities: probabilities,
		Confidence:    answer.Confidence,
	})
}

func stateAndInstructions(arguments *tools.Arguments) (string, string, error) {
	state, err := arguments.GetString(stateArg)
	if err != nil {
		return "", "", err
	}

	instructions, err := arguments.GetString(instructionsArg)
	if err != nil {
		return "", "", err
	}

	return state, instructions, nil
}

func optionalString(arguments *tools.Arguments, name string) (string, error) {
	if !arguments.Exists(name) {
		return "", nil
	}

	return arguments.GetString(name)
}

func askOne[T decision.Answer](ctx context.Context, client *decision.Decision, state string, question decision.Question) (T, error) {
	var zero T

	resp, err := client.Ask(ctx, decision.Request{
		State:     state,
		Questions: map[string]decision.Question{decisionQuestionID: question},
	})
	if err != nil {
		return zero, err
	}

	answer, ok := resp.Answers[decisionQuestionID].(T)
	if !ok {
		return zero, decision.ErrMissingAnswer
	}

	return answer, nil
}

func toJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
