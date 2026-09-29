// Package agent runs a multi-turn, tool-calling conversation with an LLM. An
// [Agent] sends the user's input to the model, runs the tools it asks for,
// feeds back the results, and repeats until the model gives a final answer.
// Create one with [New], start a conversation with [Agent.StartSession], which
// takes the [tools.ToolBox] the model may use, and run turns with
// [Agent.Process].
package agent
