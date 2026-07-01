# ai-toolkit

A personal, highly opinionated set of Go packages for working with chat-based
LLMs. It's built for my own use and reflects my own taste in API design — there
are more mature, better-supported libraries out there, and you should probably
reach for one of those first. But if it happens to fit your needs as-is, feel
free to use it.

Requires **Go 1.26+**. Supported providers: **OpenRouter** and **Ollama**.

```bash
go get github.com/jjmrocha/ai-toolkit
```

## `llm`

One consistent API for chatting with an LLM, calling tools, and reading token
usage — regardless of the backend provider.

```go
client, err := llm.New(llm.Config{
	Provider: llm.ProviderOpenRouter,
	APIKey:   os.Getenv("OPENROUTER_API_KEY"),
	Model:    "openai/gpt-4o",
})
if err != nil {
	log.Fatal(err)
}

reply, err := client.Chat(context.Background(), []llm.Message{
	llm.SystemMessage{Content: "You are concise."},
	llm.UserMessage{Content: "What is the capital of Portugal?"},
}, nil)
if err != nil {
	log.Fatal(err)
}

fmt.Println(reply.Content)
fmt.Printf("tokens: %d\n", reply.Stats.TotalTokens)
```

`New` builds a client from a `Config` (swap `Provider`/`Model` to change
backends — Ollama needs no API key). `Chat` sends an ordered `[]Message` and
returns the assistant's reply; pass `[]Tool` as the third argument to offer tools.

List the models the client may switch between in `Config.Models`. `AvailableModels`
returns that list, `CurrentModel` reports the active one, and `ChangeModel` switches
to another entry from the list (returning `ErrModelNotFound` for anything outside it).

## `tools`

Helpers that remove the two chores of tool calling: building parameter schemas
by hand and dispatching tool calls yourself.

```go
box := tools.NewToolBox()

box.AddTool(
	llm.Tool{
		Name:        "get_weather",
		Description: "Get the current weather for a city",
		Schema: tools.NewObjectBuilder().
			String("city", "the city to look up", true).
			Build(),
	},
	func(args map[string]any) (string, error) {
		city, err := tools.NewArguments(args).GetString("city")
		if err != nil {
			return "", err
		}
		return weatherFor(city) // your code
	},
)

reply, err := client.Chat(ctx, messages, box.GetTools())
// ...
for _, call := range reply.ToolCalls {
	msg, err := box.ExecuteTool(call) // looks up and runs the handler
	if err != nil {
		return err
	}
	messages = append(messages, *msg)
}
```

`ObjectBuilder` builds a tool's JSON Schema with a fluent API (scalars, arrays,
nested objects). `ToolBox` registers each tool with the function that runs it;
`GetTools` feeds them to `Chat` and `ExecuteTool` dispatches a requested call.
Inside a handler, `Arguments` reads decoded arguments back with typed accessors
that validate the type instead of panicking.

## `mcp`

Connects a stdio-based [MCP](https://modelcontextprotocol.io) server to a
`tools.ToolBox`, so the tools it exposes become callable like any other tool.

```go
box := tools.NewToolBox()

mcpClient, err := mcp.NewClient(ctx, mcp.ClientConfig{
	Name:    "playwright",
	Command: "npx",
	Args:    []string{"@playwright/mcp@latest"},
})
if err != nil {
	log.Fatal(err)
}
defer mcpClient.Close()

if err := mcpClient.RegisterTools(ctx, box); err != nil {
	log.Fatal(err)
}

reply, err := client.Chat(ctx, messages, box.GetTools()) // MCP tools included
```

`NewClient` launches the server as a child process and completes the handshake.
`RegisterTools` discovers the server's tools and adds them to the `ToolBox`,
namespaced as `"<Name>.<tool>"` (e.g. `playwright.browser_navigate`). `Close`
removes them and shuts the process down.

## `agent`

Ties `llm` and `tools` together into a conversation loop: send user input, run
whatever tools the model asks for, feed the results back, and repeat until the
model returns a final answer — so you don't write the call-tool-feed-back loop
yourself.

```go
agt, err := agent.New(agent.Config{MaxIterations: 10}, client, box)
if err != nil {
	log.Fatal(err)
}
defer agt.Close()

agt.StartSession("You are a helpful weather assistant.")

resp, err := agt.Process(ctx, "What should I wear in Lisbon today?")
if err != nil {
	log.Fatal(err)
}

fmt.Println(resp.Content)
fmt.Printf("%d tool calls, %d tokens\n",
	resp.Metadata.ToolCalls, resp.Metadata.TotalTokens)
```

`New` pairs an `llm` client with a `tools.ToolBox` (a nil box is treated as
empty). `StartSession` sets the system prompt; `Process` drives one turn,
returning the final reply plus token-usage and timing `Metadata`. `Config`
caps the loop with `MaxIterations`, and `CompactionThresholdPercent` sets when
the conversation is compacted: once a completed turn's total tokens cross that
percentage of the model's context window, the older turns are summarized into a
single message while the recent turns and system prompt are kept verbatim. A
failing tool is reported back to the model
so it can recover instead of aborting the turn. `ResetSession` clears the
conversation to the system prompt; `Close` ends the session. `AvailableModels`,
`CurrentModel`, and `ChangeModel` mirror the `llm` client to switch models
mid-conversation — the context window is re-derived on the next turn. Pass your
own `Feedback` to `SetFeedback` to observe lifecycle events (tool calls, session
start/reset/close); the default prints them to stdout.

## License

MIT — see [LICENSE](LICENSE).
