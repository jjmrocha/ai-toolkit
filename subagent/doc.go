// Package subagent lets a program hand a task to a second agent with its own
// conversation, and get back only the answer. A [Manager] holds the kinds of
// sub-agent the program offers, each registered with [Manager.Register]: a
// [Config] with its name, description and prompt, then the [agent.Agent] that
// runs it and the tools and skills each run starts with. [Manager.Run] starts a
// session on that agent, runs the task to its final answer and closes the
// session.
//
// A child is an [agent.Agent], so it calls tools, loads skills and compacts its
// context the way any agent does. Every run starts with a fresh conversation:
// the child sees the task it was given and nothing of the conversation that
// asked for it, or of earlier runs. Run blocks until the child answers.
//
// The program supplies everything a child uses, the agent and its model
// included, and nothing is inherited from the agent that asked. That has three
// consequences. A [tools.Interceptor] set on another ToolBox does not cover a
// child's calls, so set one on each sub-agent's ToolBox that needs a policy.
// Giving a sub-agent the ToolBox the sub-agent tool is registered in lets its
// children start children with no limit, and, if it has skills, removes that
// ToolBox's skill tools when a child ends. And two runs at the same time must
// not share an [agent.Agent], which is not safe for concurrent use, nor a
// ToolBox with skills, whose skill tools the first to end would remove.
//
// To let a model start sub-agents, register the tool with packs.SubAgentTools.
package subagent
