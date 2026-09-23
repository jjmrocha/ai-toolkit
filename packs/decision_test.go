package packs

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jjmrocha/ai-toolkit/decision"
	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/fn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type decisionServer struct {
	requests int
	question map[string]any
}

func newDecisionServer(t *testing.T, status int, reply string) (*decision.Decision, *decisionServer) {
	t.Helper()

	recorder := &decisionServer{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.requests++

		var body struct {
			Questions map[string]map[string]any `json:"questions"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			for _, question := range body.Questions {
				recorder.question = question
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)

		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(server.Close)

	client, err := decision.New(decision.Config{
		Provider: decision.ProviderOpenRouter,
		APIKey:   "sk-test",
		Model:    "typesafe/jev-1.13",
		BaseURL:  server.URL,
	})
	require.NoError(t, err)

	return client, recorder
}

func runDecisionTool(t *testing.T, client *decision.Decision, name string, args map[string]any) (string, error) {
	t.Helper()

	toolBox := tools.NewToolBox()

	pack, err := DecisionTools(toolBox, client)
	require.NoError(t, err)

	defer func() { _ = pack.Close() }()

	call := llm.ToolCall{ID: "call-1", Name: name, Arguments: args}

	message, err := toolBox.Execute(t.Context(), call)
	if err != nil {
		return "", err
	}

	return message.Content, nil
}

func TestDecisionTools(t *testing.T) {
	t.Run("registers the three tools", func(t *testing.T) {
		// given
		client, _ := newDecisionServer(t, http.StatusOK, `{}`)
		toolBox := tools.NewToolBox()
		// when
		pack, err := DecisionTools(toolBox, client)
		require.NoError(t, err)

		defer func() { _ = pack.Close() }()
		// then
		expected := []string{decisionChoiceToolName, decisionScoreToolName, decisionYesNoToolName}
		result := fn.Map(toolBox.Tools(), func(tool llm.Tool) string { return tool.Name })
		assert.Equal(t, expected, result)
	})

	t.Run("removes the three tools on close", func(t *testing.T) {
		// given
		client, _ := newDecisionServer(t, http.StatusOK, `{}`)
		toolBox := tools.NewToolBox()

		pack, err := DecisionTools(toolBox, client)
		require.NoError(t, err)
		// when
		require.NoError(t, pack.Close())
		// then
		assert.Empty(t, toolBox.Tools())
		assert.NoError(t, pack.Close())
	})

	t.Run("yes_no answers with the probability of yes", func(t *testing.T) {
		// given
		client, server := newDecisionServer(t, http.StatusOK,
			`{"answers":{"question":{"type":"noul","noul":0.87}}}`)
		args := map[string]any{
			"state":        "My checkout page shows a blank screen after I click Pay.",
			"instructions": "Is the customer reporting a defect?",
		}
		expected := map[string]any{
			"type":         "noul",
			"instructions": "Is the customer reporting a defect?",
		}
		// when
		result, err := runDecisionTool(t, client, decisionYesNoToolName, args)
		// then
		require.NoError(t, err)
		assert.JSONEq(t, `{"yes_probability":0.87}`, result)
		assert.Equal(t, expected, server.question)
	})

	t.Run("yes_no sends what yes and no mean when given", func(t *testing.T) {
		// given
		client, server := newDecisionServer(t, http.StatusOK,
			`{"answers":{"question":{"type":"noul","noul":0.2}}}`)
		args := map[string]any{
			"state":        "The page loads slowly on 3G.",
			"instructions": "Is the customer reporting a defect?",
			"true":         "Something is broken",
			"false":        "A feature request or a question",
		}
		expected := map[string]any{
			"true":  "Something is broken",
			"false": "A feature request or a question",
		}
		// when
		_, err := runDecisionTool(t, client, decisionYesNoToolName, args)
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, server.question["criteria"])
	})

	t.Run("choice answers with the option picked and the probability of each", func(t *testing.T) {
		// given
		client, server := newDecisionServer(t, http.StatusOK,
			`{"answers":{"question":{"type":"choice","choice":"payments",`+
				`"probabilities":{"payments":0.81,"frontend":0.19},"confidence":0.62}}}`)
		args := map[string]any{
			"state":        "My checkout page shows a blank screen after I click Pay.",
			"instructions": "Which team should own this ticket?",
			"options": []any{
				map[string]any{"name": "payments", "description": "Checkout, billing, or payment processing"},
				map[string]any{"name": "frontend"},
			},
		}
		expected := map[string]any{
			"type":         "choice",
			"instructions": "Which team should own this ticket?",
			"criteria": map[string]any{
				"payments": "Checkout, billing, or payment processing",
				"frontend": nil,
			},
		}
		// when
		result, err := runDecisionTool(t, client, decisionChoiceToolName, args)
		// then
		require.NoError(t, err)
		assert.JSONEq(t,
			`{"choice":"payments","probabilities":{"payments":0.81,"frontend":0.19},"confidence":0.62}`, result)
		assert.Equal(t, expected, server.question)
	})

	t.Run("score answers with the position and the probability of each level", func(t *testing.T) {
		// given
		client, server := newDecisionServer(t, http.StatusOK,
			`{"answers":{"question":{"type":"score","score":1.4,`+
				`"legend":{"0":"Next release","1":"This week","2":"Blocking revenue"},`+
				`"probabilities":{"0":0.1,"1":0.4,"2":0.5},"confidence":0.3}}}`)
		args := map[string]any{
			"state":        "My checkout page shows a blank screen after I click Pay.",
			"instructions": "How urgent is this ticket?",
			"levels":       []any{"Next release", "This week", "Blocking revenue"},
		}
		expected := map[string]any{
			"type":         "score",
			"instructions": "How urgent is this ticket?",
			"criteria":     []any{"Next release", "This week", "Blocking revenue"},
		}
		// when
		result, err := runDecisionTool(t, client, decisionScoreToolName, args)
		// then
		require.NoError(t, err)
		assert.JSONEq(t, `{"score":1.4,"probabilities":[`+
			`{"level":"Next release","probability":0.1},`+
			`{"level":"This week","probability":0.4},`+
			`{"level":"Blocking revenue","probability":0.5}],"confidence":0.3}`, result)
		assert.Equal(t, expected, server.question)
	})

	t.Run("rejects an invalid question without asking the model", func(t *testing.T) {
		testCases := []struct {
			name string
			tool string
			args map[string]any
		}{
			{
				name: "choice without options",
				tool: decisionChoiceToolName,
				args: map[string]any{"state": "s", "instructions": "i", "options": []any{}},
			},
			{
				name: "choice with a single option",
				tool: decisionChoiceToolName,
				args: map[string]any{
					"state": "s", "instructions": "i",
					"options": []any{map[string]any{"name": "payments"}},
				},
			},
			{
				name: "choice with a repeated option",
				tool: decisionChoiceToolName,
				args: map[string]any{
					"state": "s", "instructions": "i",
					"options": []any{map[string]any{"name": "payments"}, map[string]any{"name": "payments"}},
				},
			},
			{
				name: "score without levels",
				tool: decisionScoreToolName,
				args: map[string]any{"state": "s", "instructions": "i", "levels": []any{}},
			},
			{
				name: "score with a single level",
				tool: decisionScoreToolName,
				args: map[string]any{"state": "s", "instructions": "i", "levels": []any{"Low"}},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				// given
				client, server := newDecisionServer(t, http.StatusOK, `{}`)
				// when
				_, err := runDecisionTool(t, client, testCase.tool, testCase.args)
				// then
				require.ErrorIs(t, err, ErrInvalidQuestion)
				assert.Zero(t, server.requests)
			})
		}
	})

	t.Run("rejects a call without a state without asking the model", func(t *testing.T) {
		// given
		client, server := newDecisionServer(t, http.StatusOK, `{}`)
		args := map[string]any{"instructions": "Is the customer reporting a defect?"}
		// when
		_, err := runDecisionTool(t, client, decisionYesNoToolName, args)
		// then
		require.ErrorIs(t, err, tools.ErrFieldNotFound)
		assert.Zero(t, server.requests)
	})

	t.Run("returns the error the decision model reports", func(t *testing.T) {
		// given
		client, _ := newDecisionServer(t, http.StatusBadRequest,
			`{"error":{"message":"instructions must not be empty"}}`)
		args := map[string]any{"state": "s", "instructions": "i"}
		// when
		_, err := runDecisionTool(t, client, decisionYesNoToolName, args)
		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "400")
	})
}
