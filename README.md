# ai-toolkit

[![Go Reference](https://pkg.go.dev/badge/github.com/jjmrocha/ai-toolkit.svg)](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit)

A personal, highly opinionated set of Go packages for working with chat-based
LLMs. It's built for my own use and reflects my own taste in API design — there
are more mature, better-supported libraries out there, and you should probably
reach for one of those first. But if it happens to fit your needs as-is, feel
free to use it.

Requires **Go 1.26.5+**. Supported providers: **OpenRouter**, **Ollama**, and
**Anthropic**.

```bash
go get github.com/jjmrocha/ai-toolkit
```

| Package | What it does | Builds on |
| --- | --- | --- |
| [`llm`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/llm) | One chat API across three providers | — |
| [`tools`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/tools) | Registers tools and dispatches the model's calls | `llm` |
| [`mcp`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/mcp) | Turns an MCP server's tools into `tools` entries | `llm`, `tools` |
| [`skills`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/skills) | On-demand instructions the model loads by name | `llm`, `tools` |
| [`agent`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/agent) | Runs the call-tool-feed-back loop for you | `llm`, `tools`, `skills` |

The sections below are a tour. The full API reference lives on
[pkg.go.dev](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit).

## `llm`

One API for chatting with OpenRouter, Ollama, or Anthropic — swap `Provider` and
`Model` to change backends.

```go
model, err := llm.New(llm.Config{
	Provider: llm.ProviderOpenRouter,
	APIKey:   os.Getenv("OPENROUTER_API_KEY"),
	Model:    "openai/gpt-4o",
})
if err != nil {
	log.Fatal(err)
}

reply, err := model.Chat(context.Background(), []llm.Message{
	llm.SystemMessage{Content: "You are concise."},
	llm.UserMessage{Content: "What is the capital of Portugal?"},
}, nil)
if err != nil {
	log.Fatal(err)
}

fmt.Println(reply.Content)
fmt.Printf("tokens: %d\n", reply.Stats.TotalTokens)
```

Worth knowing:

- Ollama needs no API key.
- Every reply carries `Stats` — including prompt-cache reads and writes — and the provider's native `StopReason`.
- `Config.Effort` maps one knob, `EffortOff` through `EffortMax`, onto Anthropic's thinking-token budget and OpenRouter/Ollama's reasoning level.
- `Config.Models` lists what `ChangeModel` may switch to mid-conversation; the active model is always included.

## `tools`

Removes the two chores of tool calling: writing parameter schemas by hand and
dispatching the model's calls yourself.

```go
toolBox := tools.NewToolBox()

toolBox.Add(
	llm.Tool{
		Name:        "get_weather",
		Description: "Get the current weather for a city",
		Schema: tools.NewObjectBuilder().
			String("city", "the city to look up", true).
			Build(),
	},
	func(ctx context.Context, args map[string]any) (string, error) {
		city, err := tools.NewArguments(args).GetString("city")
		if err != nil {
			return "", err
		}
		return weatherFor(ctx, city) // your code
	},
)

reply, err := model.Chat(ctx, messages, toolBox.Tools())
// ...
for _, call := range reply.ToolCalls {
	msg, err := toolBox.Execute(ctx, call) // looks up and runs the handler
	if err != nil {
		return err
	}
	messages = append(messages, *msg)
}
```

Worth knowing:

- `Tools` returns a name-sorted slice, so the tool section of the prompt stays byte-identical across requests — which is what prompt caching needs.
- A `ToolBox` is safe for concurrent use: tools can be added and removed while other goroutines list or execute them.
- `ObjectBuilder` nests — pass one to `Object` or `ArrayOfObjects` to describe schemas of any depth.
- `Arguments` accessors return `ErrFieldNotFound` or `ErrInvalidFieldType` instead of panicking, and take an `int` where JSON handed you a `float64`.
- `ValidToolName` and `SanitizeToolName` apply the providers' naming rules (64 characters; letters, digits, `_`, `-`) to names from outside sources.

## `mcp`

Connects a stdio-based [MCP](https://modelcontextprotocol.io) server to a
`tools.ToolBox`, so the tools it exposes become callable like any other tool.

```go
toolBox := tools.NewToolBox()

mcpClient, err := mcp.NewClient(ctx, mcp.ClientConfig{
	Name:    "playwright",
	Command: "npx",
	Args:    []string{"@playwright/mcp@latest"},
})
if err != nil {
	log.Fatal(err)
}
defer mcpClient.Close()

if err := mcpClient.RegisterTools(ctx, toolBox); err != nil {
	log.Fatal(err)
}

reply, err := model.Chat(ctx, messages, toolBox.Tools()) // MCP tools included
```

Worth knowing:

- Tools are namespaced `"<Name>__<tool>"`, e.g. `playwright__browser_navigate`. A namespaced name the providers would reject is rewritten rather than dropped; the server is still called by the name it published.
- Requests are matched to responses by id, so several may be in flight at once. One blocked on a silent server returns when its context is cancelled or its deadline expires.
- `Close` shuts the process down and removes the tools it registered, aborting any call still waiting on the server.
- `Command` and `Args` are run without a shell, but they are still trusted input: supply them from operator configuration, never from an untrusted source.

### `Manager`

Runs several MCP servers on demand against a shared `ToolBox` — for example, to
expose a server's tools only while a user has it switched on.

```go
manager := mcp.NewManager(toolBox)
manager.Register(mcp.ClientConfig{
	Name:    "playwright",
	Command: "npx",
	Args:    []string{"@playwright/mcp@latest"},
})
defer manager.Close()

if err := manager.Start(ctx, "playwright"); err != nil {
	log.Fatal(err)
}

for _, status := range manager.Status() {
	fmt.Printf("%s active=%t\n", status.Name, status.Active)
}

manager.Stop("playwright") // tools removed, config kept for a later Start
```

`Register` records a launch configuration without starting it. `Start` and `Stop`
bring a server up and down by name, keeping the configuration for a later
restart; a server whose process has died is replaced on the next `Start`.
`Status` reports which are running and `Close` stops everything. Safe for
concurrent use.

## `skills`

A skill is a folder with a `SKILL.md` inside: frontmatter carrying a `name` and a
`description`, and a body holding the instructions. You add the folders a session
should have — nothing is discovered automatically.

```go
collection := skills.NewCollection()
if err := collection.Add("./skills/git-release"); err != nil {
	log.Fatal(err)
}
```

```markdown
---
name: git-release
description: Draft release notes and propose a version bump
---

Read the merged PRs since the last tag, then ...
```

Worth knowing:

- Only names and descriptions reach the model up front, as an `<available_skills>` block appended to the session's system prompt. Bodies load on demand, so a long skill costs nothing until it is used.
- Two tools are registered for the session: `skill_load` returns a skill's instructions plus the list of files it ships, and `skill_load_file` returns one of those files.
- **`skill_load` and `skill_load_file` are reserved tool names.** A tool already registered under either is replaced while the session lasts, and removed when it ends.
- An agent wires the collection up on `StartSession`; on its own, `RegisterTools` adds the two tools to any `ToolBox` and `UnregisterTools` takes them back out. `Catalog` renders the `<available_skills>` block, and `Skills` lists the names added so far, sorted.
- File access is confined to the skill folder with `os.OpenRoot`, so a symlink pointing outside it is neither listed nor readable, and the model is never told the folder's real path.
- The body and the file list are read once, by `Add`. Editing a skill on disk does not change a collection already built.
- Frontmatter keys other than `name` and `description` are ignored. Values must be single-line; a folded or literal block scalar is rejected with `ErrInvalidFrontmatter`.
- The catalog is sorted by name, so the system prompt stays byte-identical across sessions built from the same collection — which is what prompt caching needs.

## `agent`

Ties `llm` and `tools` into a conversation loop: send user input, run whatever
tools the model asks for, feed the results back, and repeat until the model
returns a final answer — so you don't write that loop yourself.

```go
agt, err := agent.New(agent.Config{MaxIterations: 10}, model)
if err != nil {
	log.Fatal(err)
}
defer agt.Close()

agt.StartSession(agent.SessionConfig{
	Prompt:  "You are a helpful weather assistant.",
	ToolBox: toolBox,
	Skills:  collection,
})

resp, err := agt.Process(ctx, "What should I wear in Lisbon today?")
if err != nil {
	log.Fatal(err)
}

fmt.Println(resp.Content)
fmt.Printf("%d tool calls, %d tokens\n",
	resp.Metadata.ToolCalls, resp.Metadata.TotalTokens)
```

Worth knowing:

- A failing tool is reported back to the model as its error text, so the model can recover instead of the turn aborting.
- Once a completed turn crosses `Config.CompactionThresholdPercent` of the model's context window (85% by default), the older turns are summarized into a single message while the system prompt and recent turns are kept verbatim.
- `Config.MaxIterations` caps the model/tool rounds per `Process` call; zero means no limit, and hitting the cap returns `ErrMaxIterations`.
- `Response.Metadata` reports token usage, stop reason, per-phase timing, and iteration and tool-call counts.
- `StartSession` declares everything the model sees: the system prompt, the `ToolBox` it may call, and the `skills.Collection` it may load from. All three last until `Close` or the next `StartSession`, so one agent can run differently equipped sessions.
- A `SessionConfig.Skills` collection has its tools registered in the session's `ToolBox` and its catalog appended to the prompt; `Close` removes those tools again.
- Install a `Feedback` sink with `SetFeedback` to observe tool calls and session events; the default is silent.

## License

MIT — see [LICENSE](LICENSE).
