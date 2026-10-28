# ai-toolkit

[![Go Reference](https://pkg.go.dev/badge/github.com/jjmrocha/ai-toolkit.svg)](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit)
![Go](https://img.shields.io/badge/go-1.27.1%2B-00ADD8?logo=go)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

**Go packages for LLM applications: one chat API, tools, MCP, skills, and an
agent loop that uses them.**

I built this toolkit for my own projects, and it follows my own preferences.
More mature libraries exist. If this one fits your needs, use it.

```bash
go get github.com/jjmrocha/ai-toolkit
```

## Highlights

- 🔌 **One API, three providers**: OpenRouter, Ollama and Anthropic.
- 🎯 **Typed decisions**: a model answers yes/no, choice and score questions with probabilities, not text.
- 🛠️ **Tools**: a schema builder, typed argument access, and one place to approve every call.
- 🧩 **MCP servers as tools**: start a stdio server, and its tools join your other tools.
- 📚 **Skills on demand**: the model loads a skill's instructions only when it needs them.
- 📦 **Ready-made tool packs**: web, code, shell, files, dates and more, in one call each.
- 🔁 **An agent loop**: it runs tool calls, compacts the context and resumes saved sessions.
- 👥 **Sub-agents with their own context**: hand a task to a second agent and get back only the answer.

## Quick start

An agent that knows today's date, running on a local Ollama model:

```go
func run(ctx context.Context) error {
	modelCfg := llm.Config{
		Provider: llm.ProviderOllama,
		Model:    "qwen3:8b",
	}

	model, err := llm.New(modelCfg)
	if err != nil {
		return err
	}

	toolBox := tools.NewToolBox()

	datePack, err := packs.DateTools(toolBox)
	if err != nil {
		return err
	}
	defer datePack.Close()

	agentCfg := agent.Config{MaxIterations: 10}

	agt, err := agent.New(agentCfg, model)
	if err != nil {
		return err
	}
	defer agt.Close()

	session := agent.SessionConfig{
		Prompt:  "You are a helpful assistant.",
		ToolBox: toolBox,
	}
	agt.StartSession(session)

	resp, err := agt.Process(ctx, "How many days until Christmas?")
	if err != nil {
		return err
	}

	fmt.Println(resp.Content)

	return nil
}
```

## Packages

| Package | What it does |
| --- | --- |
| [`llm`](#llm) | One chat API across three providers |
| [`decision`](#decision) | Typed questions answered with probabilities, not text |
| [`tools`](#tools) | Registers tools and runs the model's calls |
| [`mcp`](#mcp) | Turns an MCP server's tools into `tools` entries |
| [`skills`](#skills) | Instructions the model loads by name, when it needs them |
| [`packs`](#packs) | Ready-made tool bundles, registered in one call |
| [`agent`](#agent) | Runs the model's tool calls until the model answers |
| [`subagent`](#subagent) | Named sub-agents a model can hand tasks to |

The full API reference is on
[pkg.go.dev](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit).

### `llm`

```go
cfg := llm.Config{
	Provider: llm.ProviderOpenRouter,
	APIKey:   os.Getenv("OPENROUTER_API_KEY"),
	Model:    "openai/gpt-4o",
}

model, err := llm.New(cfg)
if err != nil {
	return err
}

messages := []llm.Message{
	llm.SystemMessage{Content: "You are concise."},
	llm.UserMessage{Content: "What is the capital of Portugal?"},
}

reply, err := model.Chat(ctx, messages, nil)
if err != nil {
	return err
}

fmt.Println(reply.Content)
```

- To use a different provider, change `Provider` and `Model`. Ollama needs no API key.
- `Effort` sets how much the model reasons, from `EffortOff` to `EffortMax`. Each provider gets its equivalent setting.
- To change the effort during a conversation, call `ChangeEffort`. To change the model, call `ChangeModel` with a model from `Config.Models`.
- Each reply contains the token usage, the prompt-cache reads and writes, and the provider's stop reason.

### `decision`

```go
cfg := decision.Config{
	Provider: decision.ProviderOpenRouter,
	APIKey:   os.Getenv("OPENROUTER_API_KEY"),
	Model:    "typesafe/jev-1.13",
}

model, err := decision.New(cfg)
if err != nil {
	return err
}

team := decision.Choice{
	Instructions: "Which team should own this ticket?",
	Options: map[string]string{
		"payments": "Checkout, billing, or payment processing",
		"frontend": "Rendering, layout, or browser compatibility",
	},
}

req := decision.Request{
	State:     "My checkout page shows a blank screen after I click Pay.",
	Questions: map[string]decision.Question{"team": team},
}

resp, err := model.Ask(ctx, req)
if err != nil {
	return err
}

answer := resp.Answers["team"].(decision.ChoiceAnswer)
fmt.Println(answer.Selected, answer.Confidence)
```

- There are three question types: `YesNo`, `Choice` and `Score`. Each one has its own answer type.
- Each answer contains probabilities. You decide which threshold to apply.
- One request can ask several questions about the same state. The model answers them in parallel.
- You can use any OpenRouter decision model as `Model`. To list these models, send `GET https://openrouter.ai/api/v1/models?output_modalities=decisions`. The default model list does not include them.

### `tools`

```go
func weather(ctx context.Context, args map[string]any) (string, error) {
	city, err := tools.NewArguments(args).GetString("city")
	if err != nil {
		return "", err
	}

	return weatherFor(ctx, city)
}
```

```go
weatherTool := llm.Tool{
	Name:        "get_weather",
	Description: "Get the current weather for a city",
	Schema: tools.NewObjectBuilder().
		String("city", "The city to look up", true).
		Build(),
}

toolBox := tools.NewToolBox()
if err := toolBox.Add(weatherTool, weather); err != nil {
	return err
}

reply, err := model.Chat(ctx, messages, toolBox.Tools())
if err != nil {
	return err
}

messages = append(messages, *reply)

for _, call := range reply.ToolCalls {
	result, err := toolBox.Execute(ctx, call)
	if err != nil {
		return err
	}

	messages = append(messages, *result)
}
```

- `ObjectBuilder` builds JSON Schemas of any depth.
- `Arguments` reads a call's arguments with typed accessors. The accessors return errors and never panic.
- `SetInterceptor` sets a function that runs before every tool call, MCP and pack tools included. Use it to ask for approval or to keep an audit trail.
- `Tools` returns the tools sorted by name. The tool list is then the same on each request, so the provider can cache the prompt.

### `mcp`

```go
cfg := mcp.ClientConfig{
	Name:    "playwright",
	Command: "npx",
	Args:    []string{"@playwright/mcp@latest"},
}

client, err := mcp.NewClient(ctx, cfg, toolBox)
if err != nil {
	return err
}
defer client.Close()
```

- The client registers each server tool as `<Name>__<tool>`. When the server's tool list changes, the client updates the registered tools.
- `Close` stops the server process and removes its tools. If the server disconnects, the client also removes its tools.
- The server gets a filtered set of environment variables. To give it other variables, name them in `InheritEnv`.
- `Manager` starts and stops several servers by name. All their tools go into one `ToolBox`.

### `skills`

A skill is a folder that contains a `SKILL.md` file:

```markdown
---
name: git-release
description: Draft release notes and propose a version bump
---

Read the merged PRs since the last tag, then ...
```

```go
collection := skills.NewCollection()

if err := collection.Add("./skills/git-release"); err != nil {
	return err
}

session := agent.SessionConfig{
	Prompt:  "You are a release assistant.",
	ToolBox: toolBox,
	Skills:  collection,
}
agt.StartSession(session)
```

- At the start, the model gets only the name and description of each skill. A skill's body adds no tokens until the model loads it.
- A skill can include files and scripts. The model can read the files and run the scripts.
- `AddClaudeSkill` adds a skill by name from `~/.claude/skills`.

### `packs`

```go
pack, err := packs.WebTools(ctx, toolBox)
if err != nil {
	return err
}
defer pack.Close()
```

| Pack | Gives the model | Needs |
| --- | --- | --- |
| `WebTools` | Web search, page fetching and crawling, via [DonSeTch](https://github.com/dondai44423/donsetch) | `npx` on `PATH` |
| `CodingTools` | Symbol-aware code navigation and editing in one project, via [Serena](https://github.com/oraios/serena). The model cannot switch to another project. Read-only mode registers only the reading and navigation tools | `uvx` on `PATH`; a project folder |
| `ThinkingTools` | Step-by-step reasoning it can revise and branch, via [sequential-thinking](https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking) | `npx` on `PATH`; Node.js 24 or newer |
| `LibraryTools` | Current documentation for libraries and frameworks, via [context7](https://github.com/upstash/context7) | `npx` on `PATH`; Node.js 20.18.1 or newer; `CONTEXT7_API_KEY` optional |
| `FinanceTools` | Market data from Yahoo Finance: quotes, price history, financial statements, analyst estimates, news, holders, funds, options and screeners, via [yfmcp](https://github.com/narumiruna/yfinance-mcp) | `uvx` on `PATH` |
| `ShellTools` | A `/bin/sh` command runner | — |
| `FileTools` | The files in one folder, and no files outside it. Read-only mode registers only reading, listing and searching | — |
| `DateTools` | The date, time of day and time zone | — |
| `DecisionTools` | Typed questions to a decision model, answered with probabilities | A `decision.Decision` |
| `SubAgentTools` | A `subagent` tool that hands a task to another agent | A `subagent.Manager` |

- Always call `Close` when you no longer need a pack. It removes the pack's tools and stops the server the pack started, if any.
- `Instructions` returns the pack's usage instructions for the system prompt. It returns nil when the pack has none.

### `agent`

```go
agentCfg := agent.Config{MaxIterations: 10}

agt, err := agent.New(agentCfg, model)
if err != nil {
	return err
}
defer agt.Close()

session := agent.SessionConfig{
	Prompt:  "You are a helpful weather assistant.",
	ToolBox: toolBox,
}
agt.StartSession(session)

resp, err := agt.Process(ctx, "What should I wear in Lisbon today?")
if err != nil {
	return err
}

fmt.Println(resp.Content)
```

- The agent runs the model's tool calls and sends the results back to the model. It continues until the model gives a final answer.
- When a tool fails, the agent sends the error text to the model, so the model can recover.
- Before the context window is full, the agent summarizes the older turns.
- To save a session, call `Messages`. To resume it, pass those messages in `SessionConfig.Messages`.
- `Feedback` reports tool calls and session events. To handle only some events, embed `NopFeedback` in your type.

### `subagent`

```go
explorerAgent, err := agent.New(agent.Config{MaxIterations: 20}, model)
if err != nil {
	return err
}

manager := subagent.NewManager()

explorer := subagent.Config{
	Name:        "explore",
	Description: "Finds code and answers questions about the codebase",
	Prompt:      "You explore a codebase and report what you find.",
}

if err := manager.Register(explorer, explorerAgent, readOnlyTools, nil); err != nil {
	return err
}

pack, err := packs.SubAgentTools(toolBox, manager)
if err != nil {
	return err
}
defer pack.Close()
```

- Each sub-agent has its own `agent.Agent`, prompt, tools and skills. It inherits nothing from the agent that calls it.
- Each run starts a new conversation and closes it after the final answer. The calling agent gets only the answer, so its context stays small.
- The `subagent` tool's description lists the sub-agents, and the model picks one by name. Register every sub-agent before you call `SubAgentTools`.
- To start a sub-agent from your own code, call `Run`.
- `SetFeedback` installs a `Feedback` that reports the start, end, tool calls and token usage of each run. Each event includes the sub-agent's name.
- `Run` replaces the `agent.Feedback` of the sub-agent's agent.

## Security

`ShellTools`, `CodingTools`, skill scripts and MCP server commands run with
your program's authority over the whole filesystem. `ShellTools` also inherits
your environment and any credentials in it. MCP servers and skill scripts get a
filtered environment, but they can still read credential files on disk.

Register these tools only for a model and a conversation that you trust with
this access. Take MCP commands and skill folders from your own configuration.
Never take them from untrusted input.

Each sub-agent uses its own `ToolBox`. An interceptor on the main `ToolBox`
does not see the sub-agent's tool calls. If a sub-agent needs a policy, set an
interceptor on that sub-agent's `ToolBox`. The sub-agent `Feedback` does not
report tool arguments, so use that interceptor to audit them.

## License

MIT, see [LICENSE](LICENSE).
