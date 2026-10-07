# ai-toolkit

[![Go Reference](https://pkg.go.dev/badge/github.com/jjmrocha/ai-toolkit.svg)](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit)
![Go](https://img.shields.io/badge/go-1.27.1%2B-00ADD8?logo=go)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

**Go building blocks for LLM apps: one chat API, tools, MCP, skills, and an
agent loop that ties them together.**

A personal, opinionated toolkit built for my own projects. More mature
libraries exist, but if this one fits, feel free to use it.

```bash
go get github.com/jjmrocha/ai-toolkit
```

## Highlights

- 🔌 **One API, three providers**: OpenRouter, Ollama and Anthropic.
- 🛠️ **Tools without the chores**: schema builder, typed argument access, one place to approve every call.
- 🧩 **MCP servers as tools**: launch a stdio server and its tools join the rest.
- 📚 **Skills on demand**: the model loads a skill's instructions only when it needs them.
- 📦 **Ready-made tool packs**: web, code, shell, files, dates and more, in one call each.
- 🔁 **An agent loop you don't write**: tool calls, context compaction and resumable sessions.

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
| [`classify`](#classify) | Typed questions answered with probabilities, not text |
| [`tools`](#tools) | Registers tools and runs the model's calls |
| [`mcp`](#mcp) | Turns an MCP server's tools into `tools` entries |
| [`skills`](#skills) | Instructions the model loads by name, when it needs them |
| [`packs`](#packs) | Ready-made tool bundles, registered in one call |
| [`agent`](#agent) | The call-tool-feed-back loop, done for you |

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

- Switch backends by changing `Provider` and `Model`. Ollama needs no API key.
- `Effort` sets how much the model reasons, from `EffortOff` to `EffortMax`, translated for each provider.
- Switch effort mid-conversation with `ChangeEffort`, or the model with `ChangeModel` among those listed in `Config.Models`.
- Every reply carries token usage, prompt-cache hits included, and the provider's stop reason.

### `classify`

```go
cfg := classify.Config{
	Provider: classify.ProviderOpenRouter,
	APIKey:   os.Getenv("OPENROUTER_API_KEY"),
	Model:    "typesafe/jev-1.13",
}

classifier, err := classify.New(cfg)
if err != nil {
	return err
}

team := classify.Choice{
	Instructions: "Which team should own this ticket?",
	Options: map[string]string{
		"payments": "Checkout, billing, or payment processing",
		"frontend": "Rendering, layout, or browser compatibility",
	},
}

req := classify.Request{
	Input:     "My checkout page shows a blank screen after I click Pay.",
	Questions: map[string]classify.Question{"team": team},
}

resp, err := classifier.Classify(ctx, req)
if err != nil {
	return err
}

answer := resp.Answers["team"].(classify.ChoiceAnswer)
fmt.Println(answer.Selected, answer.Confidence)
```

- Three question types: `YesNo`, `Choice` and `Score`, each with a typed answer.
- Answers carry calibrated probabilities. What threshold to apply is your call.
- Several questions about one input are answered in parallel, in one request.

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

for _, call := range reply.ToolCalls {
	result, err := toolBox.Execute(ctx, call)
	if err != nil {
		return err
	}

	messages = append(messages, *result)
}
```

- `ObjectBuilder` writes JSON Schemas of any depth.
- `Arguments` reads a call's arguments with typed accessors that return errors, never panic.
- `SetInterceptor` runs before every tool call, however the tool was registered: the one place to ask for approval or keep an audit trail.
- `Tools` is sorted by name, so the prompt stays cache-friendly.

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

- The server's tools are registered as `<Name>__<tool>` and follow the server's tool list as it changes.
- `Close` stops the process and removes its tools; a server that dies removes them too.
- The server gets a filtered environment. Pass anything else it needs with `InheritEnv`.
- `Manager` starts and stops several servers by name against one `ToolBox`.

### `skills`

A skill is a folder with a `SKILL.md`:

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

- Only names and descriptions reach the model up front. A skill's body costs nothing until it's loaded.
- Skills can ship files and scripts the model reads or runs.
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
| `WebTools` | Web search, page fetching and crawling, via [DonSeTch](https://github.com/dondai44423/donsetch) | `donsetch` on `PATH` |
| `CodingTools` | Symbol-aware code navigation and editing, via [Serena](https://github.com/oraios/serena) | `uvx` on `PATH` |
| `ThinkingTools` | Step-by-step reasoning it can revise and branch, via [sequential-thinking](https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking) | `npx` on `PATH` |
| `ShellTools` | A `/bin/sh` command runner | — |
| `FileTools` | The files under one folder it can't leave | — |
| `DateTools` | The date, time of day and time zone | — |
| `ClassifyTools` | Judgement calls with calibrated probabilities | A `classify.Classifier` |

- `Close` removes the pack's tools and stops any server behind them. Always call it.
- `Instructions` returns the pack's usage guidance for the system prompt.

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

- Runs the model's tool calls and feeds the results back until it answers. A failing tool becomes error text the model can recover from.
- Summarizes older turns automatically before the context window fills up.
- Sessions can be saved with `Messages` and resumed through `SessionConfig`.
- `Feedback` follows tool calls and session events. Embed `NopFeedback` to handle only the ones you need.

## Security

`ShellTools`, `CodingTools`, skill scripts and MCP server commands run with
your program's authority: the whole filesystem, the environment and any
credentials in it. Register them only for a model and conversation you'd trust
with that, and take MCP commands and skill folders from your own
configuration, never from untrusted input.

## License

MIT, see [LICENSE](LICENSE).
