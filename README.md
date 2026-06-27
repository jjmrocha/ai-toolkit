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
	Name:    "files",
	Command: "mcp-server-filesystem",
	Args:    []string{"/data"},
}, box)
if err != nil {
	log.Fatal(err)
}
defer mcpClient.Close()

if err := mcpClient.RegisterTools(ctx); err != nil {
	log.Fatal(err)
}

reply, err := client.Chat(ctx, messages, box.GetTools()) // MCP tools included
```

`NewClient` launches the server as a child process and completes the handshake.
`RegisterTools` discovers the server's tools and adds them to the `ToolBox`,
namespaced as `"<Name>.<tool>"` (e.g. `files.read_file`). `Close` removes them
and shuts the process down.

## License

MIT — see [LICENSE](LICENSE).
