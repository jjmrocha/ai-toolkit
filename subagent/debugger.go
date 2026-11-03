package subagent

import "github.com/jjmrocha/ai-toolkit/llm"

// Debugger receives the conversation of each sub-agent run, so a host can save
// it and review how the sub-agent followed its instructions. Install one with
// [Manager.SetDebugger].
//
// agentName is the name the run was started with. messages is a copy of the
// child's conversation, as [agent.Agent.Messages] returns it: the system
// message, the task, then every reply, tool call and tool result. It ends with
// the final answer when the run succeeds, and wherever the child stopped when
// it fails. The Debugger may keep it. err is nil on success, or the error
// [Manager.Run] returns, such as [agent.ErrMaxIterations].
//
// A run that [Manager.Run] rejects before it starts the child, for an empty
// prompt or an unknown name, has no conversation and does not call it.
//
// [Manager.Run] calls it before it returns, so a slow Debugger delays the
// answer. Runs can happen at the same time, so it must be safe for concurrent
// use.
type Debugger func(agentName string, messages []llm.Message, err error)
