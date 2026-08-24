// Package agent drives a multi-turn, tool-calling conversation with an LLM. An
// [Agent] sends user input to the model, runs any tools the model requests,
// feeds the results back, and repeats until the model returns a final answer.
// Construct one from an [llm.LLM] with [New], begin a conversation with
// [Agent.StartSession] — which takes the [tools.ToolBox] the session may call —
// then drive turns with [Agent.Process].
package agent
