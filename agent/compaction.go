package agent

import (
	"fmt"
	"strings"

	"github.com/jjmrocha/ai-toolkit/llm"
)

const (
	defaultKeepRecentTurns            = 1
	defaultCompactionThresholdPercent = 85
	summaryPrefix                     = "Summary of the conversation so far:\n"
	summarySystemPrompt               = "You compress a conversation into a concise summary. " +
		"Preserve the facts, decisions, and tool results needed to continue the task. " +
		"Reply with the summary only."
)

func compactionThreshold(contextSize, pct int) int {
	if pct == 0 {
		pct = defaultCompactionThresholdPercent
	}

	return contextSize * pct / 100
}

func indexOfTheBeginningOfTurnToKeep(messages []llm.Message) int {
	lastIndex := len(messages) - 1
	var turnsFound int

	for i := len(messages) - 1; i >= 0; i-- {
		if _, ok := messages[i].(llm.UserMessage); ok {
			lastIndex = i
			turnsFound++

			if turnsFound == defaultKeepRecentTurns {
				return lastIndex
			}
		}
	}

	return lastIndex
}

func renderConversation(messages []llm.Message) string {
	var b strings.Builder
	for _, m := range messages {
		switch v := m.(type) {
		case llm.SystemMessage:
			fmt.Fprintf(&b, "System: %s\n", v.Content)
		case llm.UserMessage:
			fmt.Fprintf(&b, "User: %s\n", v.Content)
		case llm.AssistantMessage:
			if v.Content != "" {
				fmt.Fprintf(&b, "Assistant: %s\n", v.Content)
			}
			for _, tc := range v.ToolCalls {
				fmt.Fprintf(&b, "Assistant called tool %s with %v\n", tc.Name, tc.Arguments)
			}
		case llm.ToolMessage:
			fmt.Fprintf(&b, "Tool %s result: %s\n", v.ToolName, v.Content)
		}
	}
	return b.String()
}
