package subagent

import "github.com/jjmrocha/ai-toolkit/llm"

// Debugger receives the conversation of each successful sub-agent run, so a
// host can save it and review how the sub-agent followed its instructions.
// Install one with [Manager.SetDebugger].
//
// agentName is the name the run was started with. messages is a copy of the
// child's conversation, as [agent.Agent.Messages] returns it: the system
// message, the task, then every reply, tool call and tool result up to the
// final answer. The Debugger may keep it.
//
// [Manager.Run] calls it before it returns, so a slow Debugger delays the
// answer. Runs can happen at the same time, so it must be safe for concurrent
// use.
type Debugger func(agentName string, messages []llm.Message)
