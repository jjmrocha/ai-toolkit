package packs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jjmrocha/ai-toolkit/classify"
	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	classifyYesNoToolName  = "classify_yes_no"
	classifyChoiceToolName = "classify_choice"
	classifyScoreToolName  = "classify_score"
	inputArg               = "input"
	instructionsArg        = "instructions"
	trueArg                = "true"
	falseArg               = "false"
	optionsArg             = "options"
	optionNameArg          = "name"
	optionDescriptionArg   = "description"
	levelsArg              = "levels"
	classifyQuestionID     = "question"
)

var classifyToolNames = []string{
	classifyYesNoToolName,
	classifyChoiceToolName,
	classifyScoreToolName,
}

type classifyPack struct {
	toolBox *tools.ToolBox
	once    sync.Once
}

func (p *classifyPack) Close() error {
	p.once.Do(func() {
		for _, name := range classifyToolNames {
			p.toolBox.Remove(name)
		}
	})

	return nil
}

type yesNoResult struct {
	YesProbability float64 `json:"yes_probability"`
}

type choiceResult struct {
	Selected      string             `json:"selected"`
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

// ClassifyTools registers the three tools that let the model hand a judgement
// call to the classification model behind client, one question per call:
// "classify_yes_no" asks a [classify.YesNo], "classify_choice" a
// [classify.Choice] and "classify_score" a [classify.Score]. Each takes the
// input to judge and the question's instructions, and returns the answer as
// JSON — the probability of yes, the option picked with the probability of each,
// or the position across the levels with the probability of each. The returned
// [ToolPack] removes the three again.
//
// Whatever the model puts in the input is sent to the classification provider,
// and every call is billed on its input tokens, the whole input included.
//
// A choice with fewer than two options or the same option twice, and a score
// with fewer than two levels, fail with [ErrInvalidQuestion] before the provider
// is called. An error from [classify.Classifier.Classify] is returned as it is.
//
// The caller owns client: [ToolPack.Close] only unregisters the tools, and a
// dropped pack costs nothing beyond the tools staying registered.
//
// It fails with the error [tools.ToolBox.Add] returns when m rejects a
// registration, which leaves any tool already registered by the call in place.
func ClassifyTools(m *tools.ToolBox, client *classify.Classifier) (ToolPack, error) {
	yesNoTool := llm.Tool{
		Name: classifyYesNoToolName,
		Description: "Ask a calibrated classifier a yes/no question about an input, instead of " +
			"judging it yourself. Returns {\"yes_probability\": p}, the probability that the answer is " +
			"yes: 0.5 means undecided, not a middle answer. The model sees only the input and the question, " +
			"so make the input self-contained.",
		Schema: tools.NewObjectBuilder().
			String(inputArg, "the content to judge", true).
			String(instructionsArg, "the yes/no question to answer about the input", true).
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
		Name: classifyChoiceToolName,
		Description: "Ask a calibrated classifier to pick one of several options for an input, " +
			"instead of choosing yourself. Returns {\"selected\", \"probabilities\", \"confidence\"}: the " +
			"most probable option, the probability of each, and how concentrated they are from 0 to 1 — " +
			"how sure the model is, not how likely it is to be right. The model sees only the input and " +
			"the question, so make the input self-contained.",
		Schema: tools.NewObjectBuilder().
			String(inputArg, "the content to judge", true).
			String(instructionsArg, "what to decide about the input", true).
			ArrayOfObjects(optionsArg, "the options to pick from", true, tools.NewObjectBuilder().
				String(optionNameArg, "the option's name, returned as selected", true).
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
		Name: classifyScoreToolName,
		Description: "Ask a calibrated classifier to rate an input against ordered levels, instead of " +
			"rating it yourself. Returns {\"score\", \"probabilities\", \"confidence\"}: a " +
			"probability-weighted position from 0 to the last level's index, which can fall between two " +
			"levels; the probability of each level; and how concentrated they are from 0 to 1. The model " +
			"sees only the input and the question, so make the input self-contained.",
		Schema: tools.NewObjectBuilder().
			String(inputArg, "the content to judge", true).
			String(instructionsArg, "what to rate about the input", true).
			ArrayOfStrings(levelsArg, "a description of each level, from lowest to highest", true).
			Build(),
	}

	err = m.Add(scoreTool, func(ctx context.Context, args map[string]any) (string, error) {
		return askScore(ctx, client, args)
	})
	if err != nil {
		return nil, err
	}

	return &classifyPack{toolBox: m}, nil
}

func askYesNo(ctx context.Context, client *classify.Classifier, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	input, instructions, err := inputAndInstructions(arguments)
	if err != nil {
		return "", err
	}

	question := classify.YesNo{Instructions: instructions}

	if question.True, err = optionalString(arguments, trueArg); err != nil {
		return "", err
	}

	if question.False, err = optionalString(arguments, falseArg); err != nil {
		return "", err
	}

	answer, err := askOne[classify.YesNoAnswer](ctx, client, input, question)
	if err != nil {
		return "", err
	}

	return toJSON(yesNoResult{YesProbability: answer.Value})
}

func askChoice(ctx context.Context, client *classify.Classifier, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	input, instructions, err := inputAndInstructions(arguments)
	if err != nil {
		return "", err
	}

	options, err := arguments.GetArrayOfObjects(optionsArg)
	if err != nil {
		return "", err
	}

	question := classify.Choice{Instructions: instructions, Options: make(map[string]string, len(options))}

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

	answer, err := askOne[classify.ChoiceAnswer](ctx, client, input, question)
	if err != nil {
		return "", err
	}

	return toJSON(choiceResult{
		Selected:      answer.Selected,
		Probabilities: answer.Probabilities,
		Confidence:    answer.Confidence,
	})
}

func askScore(ctx context.Context, client *classify.Classifier, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	input, instructions, err := inputAndInstructions(arguments)
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

	question := classify.Score{Instructions: instructions, Levels: levels}

	answer, err := askOne[classify.ScoreAnswer](ctx, client, input, question)
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

func inputAndInstructions(arguments *tools.Arguments) (string, string, error) {
	input, err := arguments.GetString(inputArg)
	if err != nil {
		return "", "", err
	}

	instructions, err := arguments.GetString(instructionsArg)
	if err != nil {
		return "", "", err
	}

	return input, instructions, nil
}

func optionalString(arguments *tools.Arguments, name string) (string, error) {
	if !arguments.Exists(name) {
		return "", nil
	}

	return arguments.GetString(name)
}

func askOne[T classify.Answer](ctx context.Context, client *classify.Classifier, input string, question classify.Question) (T, error) {
	var zero T

	resp, err := client.Classify(ctx, classify.Request{
		Input:     input,
		Questions: map[string]classify.Question{classifyQuestionID: question},
	})
	if err != nil {
		return zero, err
	}

	answer, ok := resp.Answers[classifyQuestionID].(T)
	if !ok {
		return zero, classify.ErrMissingAnswer
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
