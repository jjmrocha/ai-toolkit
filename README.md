# ai-toolkit

A small, provider-agnostic client for chat-based large language models in Go.
The `llm` package gives you one consistent API for sending messages, calling
tools, and reading token usage, regardless of the backend. The companion
`tools` package helps you register tool handlers and build their JSON Schemas.

**OpenRouter** and **Ollama** are the supported providers. The design isolates
each provider behind an internal interface, so additional backends can be added
without changing the public API.

Requires **Go 1.26+**. HTTP transport is handled by
[resty](https://github.com/go-resty/resty) (retries on `429`/`5xx` and a request
timeout are configured out of the box).

## Installation

```bash
go get github.com/jjmrocha/ai-toolkit
```

```go
import "github.com/jjmrocha/ai-toolkit/llm"
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jjmrocha/ai-toolkit/llm"
)

func main() {
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
}
```

## Concepts

**Client.** An [`LLM`](#api-reference) is created with [`New`](#api-reference) from a
[`Config`](#config). `Provider` and `Model` are always required; `APIKey` is
required for OpenRouter but unused by Ollama (typically a local server); `BaseURL`
defaults to the provider's standard endpoint. The client is safe for concurrent
use.

**Messages.** A conversation is an ordered `[]Message`. `Message` is a *sealed*
interface — only the four types in this package implement it:

| Type | Role | Purpose |
|------|------|---------|
| `SystemMessage` | `system` | Instructions that steer the model. |
| `UserMessage` | `user` | Input from the end user. |
| `AssistantMessage` | `assistant` | A reply from the model (may request tool calls; carries `Stats`). |
| `ToolMessage` | `tool` | The result of a tool call, fed back to the model. |

**Tools.** A [`Tool`](#tools) describes a function the model may call — a name, a
description, and a JSON Schema for its parameters. A tool is a *description only*;
your code executes the work and returns the result as a `ToolMessage`.

## Usage

OpenRouter (hosted, requires an API key):

```go
client, err := llm.New(llm.Config{
	Provider: llm.ProviderOpenRouter,          // required
	APIKey:   os.Getenv("OPENROUTER_API_KEY"), // required for OpenRouter
	Model:    "openai/gpt-4o",                 // required
	BaseURL:  "",                              // optional — defaults to https://openrouter.ai/api/v1
})
```

Ollama (local, no API key):

```go
client, err := llm.New(llm.Config{
	Provider: llm.ProviderOllama, // required
	Model:    "llama3.2",         // required
	BaseURL:  "",                 // optional — defaults to http://localhost:11434
})
```

`New` returns [`ErrMissingProvider`](#errors) or [`ErrMissingModel`](#errors) when a
required field is empty, [`ErrMissingAPIKey`](#errors) when OpenRouter is selected
without a key, and [`ErrUnsupportedProvider`](#errors) when the provider is not
recognized.

### Chat

`Chat` sends the conversation and returns the assistant's reply. The `context`
controls cancellation and deadline.

```go
reply, err := client.Chat(ctx, []llm.Message{
	llm.SystemMessage{Content: "You are a helpful assistant."},
	llm.UserMessage{Content: "Summarize the plot of Hamlet in one sentence."},
}, nil) // no tools
if err != nil {
	return err
}
fmt.Println(reply.Content)
```

### Tool calling

Offer tools by passing them to `Chat`. If the model decides to call one, the
reply's `ToolCalls` is non-empty; run the tool and feed the result back as a
`ToolMessage`, then call `Chat` again with the extended history.

```go
weather := llm.Tool{
	Name:        "get_weather",
	Description: "Get the current weather for a city",
	Schema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city": map[string]any{"type": "string"},
		},
		"required": []string{"city"},
	},
}

messages := []llm.Message{llm.UserMessage{Content: "What's the weather in Lisbon?"}}

reply, err := client.Chat(ctx, messages, []llm.Tool{weather})
if err != nil {
	return err
}

if len(reply.ToolCalls) > 0 {
	// Keep the assistant turn in the history...
	messages = append(messages, reply)

	for _, call := range reply.ToolCalls {
		result := runTool(call.Name, call.Arguments) // your code
		messages = append(messages, llm.ToolMessage{
			ToolCallID: call.ID,   // OpenRouter correlates by call id
			ToolName:   call.Name, // Ollama correlates by tool name
			Content:    result,
		})
	}

	// ...and ask the model to continue with the tool results.
	reply, err = client.Chat(ctx, messages, []llm.Tool{weather})
	if err != nil {
		return err
	}
}

fmt.Println(reply.Content)
```

The `Schema` must be a JSON Schema object describing the parameters. Keep to the
common subset (`type`, `properties`, `required`, `enum`, nested objects/arrays)
for portability across providers.

### The `tools` package

Writing schema maps by hand and dispatching tool calls yourself gets tedious.
The `tools` package removes both chores: `ObjectBuilder` constructs the
parameter schema, and `ToolBox` registers each tool with the function that runs
it, then dispatches a requested call for you.

```go
import "github.com/jjmrocha/ai-toolkit/tools"

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
		city, _ := args["city"].(string) // arguments arrive with JSON types
		return weatherFor(city)           // your code
	},
)

messages := []llm.Message{llm.UserMessage{Content: "What's the weather in Lisbon?"}}

reply, err := client.Chat(ctx, messages, box.GetTools()) // offer every registered tool
if err != nil {
	return err
}

if len(reply.ToolCalls) > 0 {
	messages = append(messages, reply)

	for _, call := range reply.ToolCalls {
		msg, err := box.ExecuteTool(call) // looks up and runs the handler
		if err != nil {
			return err
		}
		messages = append(messages, *msg)
	}

	reply, err = client.Chat(ctx, messages, box.GetTools())
	if err != nil {
		return err
	}
}

fmt.Println(reply.Content)
```

`ExecuteTool` returns [`ErrToolNotFound`](#errors) if the model names a tool that
was never registered. A `ToolBox` is safe for concurrent `GetTools`/`ExecuteTool`
once all tools are registered, but registration itself is not — set it up before
serving requests.

`ObjectBuilder` covers scalars (`String`, `Integer`, `Number`, `Boolean`),
arrays of each (`ArrayOfStrings`, …), and nesting. Nested objects and arrays of
objects take their own `*ObjectBuilder`, so any depth composes:

```go
address := tools.NewObjectBuilder().
	String("street", "street name", true).
	String("city", "city name", true)

schema := tools.NewObjectBuilder().
	String("name", "full name", true).
	Object("address", "mailing address", true, address).
	ArrayOfStrings("tags", "labels", false).
	Build()
```

### Model info

`ModelInfo` reports metadata about the configured model.

```go
info, err := client.ModelInfo(ctx)
if err != nil {
	return err
}
fmt.Printf("%s — %d token context\n", info.Name, info.ContextSize)
```

It returns [`ErrModelNotFound`](#errors) if the configured model is not offered
by the provider.

## API reference

For full documentation see
[pkg.go.dev/github.com/jjmrocha/ai-toolkit/llm](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/llm).

Constructor: `New(cfg Config) (*LLM, error)`.

`LLM`:

| Method | Signature | Description |
|--------|-----------|-------------|
| Chat | `Chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error)` | Send the conversation and return the assistant's reply. |
| ModelInfo | `ModelInfo(ctx context.Context) (ModelInfo, error)` | Report the configured model's name and context size. |

### Config

| Field | Type | Description |
|-------|------|-------------|
| Provider | `Provider` | Backend to use: `ProviderOpenRouter` or `ProviderOllama`. Required. |
| APIKey | `string` | Provider API key. Required for OpenRouter; unused by Ollama. |
| Model | `string` | Provider-specific model id, e.g. `"openai/gpt-4o"` or `"llama3.2"`. Required. |
| BaseURL | `string` | Override the provider endpoint. Optional (defaults per provider). |

### Messages

`Message` is a sealed interface exposing `Role() RoleName`. Implementations:

| Type | Fields |
|------|--------|
| `SystemMessage` | `Content string` |
| `UserMessage` | `Content string` |
| `AssistantMessage` | `Content string`, `ToolCalls []ToolCall`, `Stats Stats` |
| `ToolMessage` | `ToolCallID string`, `ToolName string`, `Content string` |

Role constants: `SystemRole`, `UserRole`, `AssistantRole`, `ToolRole` (type `RoleName`).

On a `ToolMessage`, set `ToolCallID` for OpenRouter (id-based correlation) or `ToolName` for Ollama (name-based).

### Tools

| Type | Fields | Description |
|------|--------|-------------|
| `Tool` | `Name`, `Description string`, `Schema map[string]any` | A function definition offered to the model. |
| `ToolCall` | `ID`, `Name string`, `Arguments map[string]any` | A tool invocation requested by the model. |

### Other types

| Type | Fields | Description |
|------|--------|-------------|
| `ModelInfo` | `Name string`, `ContextSize int` | Model metadata (`ContextSize` in tokens). |
| `Stats` | `PromptTokens`, `OutputTokens`, `TotalTokens int` | Token usage for a response. |
| `Provider` | — | Provider identifier: `ProviderOpenRouter`, `ProviderOllama`. |

### Errors

| Error | Returned by | When |
|-------|-------------|------|
| `ErrMissingProvider` | `New` | `Config.Provider` is empty. |
| `ErrMissingModel` | `New` | `Config.Model` is empty. |
| `ErrMissingAPIKey` | `New` | `Config.APIKey` is empty (OpenRouter only). |
| `ErrUnsupportedProvider` | `New` | `Config.Provider` is not recognized. |
| `ErrModelNotFound` | `ModelInfo` | The configured model is not offered by the provider. |

### `tools` package

Import `github.com/jjmrocha/ai-toolkit/tools`. Full docs at
[pkg.go.dev/github.com/jjmrocha/ai-toolkit/tools](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/tools).

`ToolBox` — registry mapping tools to their handlers:

| Method | Signature | Description |
|--------|-----------|-------------|
| NewToolBox | `NewToolBox() *ToolBox` | Create an empty registry. |
| AddTool | `AddTool(tool llm.Tool, handler Handler)` | Register a tool and its handler (keyed by `tool.Name`; re-registering replaces). |
| RemoveTool | `RemoveTool(name string)` | Unregister a tool; no-op if absent. |
| GetTools | `GetTools() []llm.Tool` | All registered tool definitions, for passing to `Chat`. |
| ExecuteTool | `ExecuteTool(call llm.ToolCall) (*llm.ToolMessage, error)` | Run the handler for a call and wrap the result; returns `ErrToolNotFound` if unknown. |

`Handler` — `func(map[string]any) (string, error)`. Receives the model's decoded
arguments (JSON types: numbers arrive as `float64`) and returns the result
string or an error.

`ObjectBuilder` — fluent builder for a parameter schema. Every `Add*` method
returns the builder for chaining; `Build() map[string]any` produces the schema:

| Method | Adds |
|--------|------|
| `String` / `Integer` / `Number` / `Boolean` | A scalar field. |
| `ArrayOfStrings` / `ArrayOfIntegers` / `ArrayOfNumbers` / `ArrayOfBooleans` | An array of that scalar. |
| `Object(name, desc string, required bool, spec *ObjectBuilder)` | A nested object. |
| `ArrayOfObjects(name, desc string, required bool, spec *ObjectBuilder)` | An array of objects. |

All `Add*` methods share the leading signature `(name, desc string, required bool)`.

| Error | Returned by | When |
|-------|-------------|------|
| `ErrToolNotFound` | `ToolBox.ExecuteTool` | No registered tool matches the call name. |

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
