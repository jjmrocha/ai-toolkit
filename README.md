# ai-toolkit

[![Go Reference](https://pkg.go.dev/badge/github.com/jjmrocha/ai-toolkit.svg)](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit)

A personal, highly opinionated set of Go packages for working with chat-based
LLMs. It's built for my own use and reflects my own taste in API design. There
are more mature, better-supported libraries out there, and you should probably
reach for one of those first. But if it happens to fit your needs as-is, feel
free to use it.

Requires **Go 1.27.1+**. Supported chat providers: **OpenRouter**, **Ollama**, and
**Anthropic**.

```bash
go get github.com/jjmrocha/ai-toolkit
```

| Package | What it does | Builds on |
| --- | --- | --- |
| [`llm`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/llm) | One chat API across three providers | |
| [`classify`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/classify) | Typed questions answered with probabilities, not text | |
| [`tools`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/tools) | Registers tools and dispatches the model's calls | `llm` |
| [`mcp`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/mcp) | Turns an MCP server's tools into `tools` entries | `llm`, `tools` |
| [`skills`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/skills) | On-demand instructions the model loads by name | `llm`, `tools` |
| [`agent`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/agent) | Runs the call-tool-feed-back loop for you | `llm`, `tools`, `skills` |
| [`packs`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/packs) | Ready-made tool bundles, registered in one call | `classify`, `mcp`, `tools` |

The sections below are a tour. The full API reference is on
[pkg.go.dev](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit).

## `llm`

One API for OpenRouter, Ollama and Anthropic. Change `Provider` and `Model` to
switch backends.

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
- Every reply carries `Stats`, prompt-cache reads and writes included, and the provider's own `StopReason`.
- `Config.Effort`, from `EffortOff` to `EffortMax`, maps onto Anthropic's adaptive-thinking effort and the OpenRouter and Ollama reasoning level. The values are relative levels, so each provider receives its own equivalent.
- `Config.Models` lists what `ChangeModel` may switch to mid-conversation. The active model is always included.
- `ChangeModel` and `ChangeEffort` validate first. A rejected change returns an error and leaves the client as it was.

## `classify`

Asks a classification model typed questions about an input and returns typed
answers, each with the probabilities behind it. There is no prose to parse. The
provider is OpenRouter, which serves TypeSafe's Jev models.

```go
client, err := classify.New(classify.Config{
	Provider: classify.ProviderOpenRouter,
	APIKey:   os.Getenv("OPENROUTER_API_KEY"),
	Model:    "typesafe/jev-1.13",
})
if err != nil {
	log.Fatal(err)
}

answers, err := client.Classify(context.Background(), classify.Request{
	Input: "My checkout page shows a blank screen after I click Pay.",
	Questions: map[string]classify.Question{
		"is_bug": classify.YesNo{Instructions: "Is the customer reporting a defect?"},
		"team": classify.Choice{
			Instructions: "Which team should own this ticket?",
			Options: map[string]string{
				"payments": "Checkout, billing, or payment processing",
				"frontend": "Rendering, layout, or browser compatibility",
			},
		},
		"urgency": classify.Score{
			Instructions: "How urgent is this ticket?",
			Levels:       []string{"Next release", "This week", "Blocking revenue"},
		},
	},
})
if err != nil {
	log.Fatal(err)
}

team := answers.Answers["team"].(classify.ChoiceAnswer)
fmt.Println(team.Selected, team.Confidence)
```

Worth knowing:

- `Request.Questions` and `Response.Answers` share the identifiers you choose. The model never sees them.
- The questions are answered in parallel, and none sees another's answer.
- `Answer` is a sealed interface (`YesNoAnswer`, `ChoiceAnswer`, `ScoreAnswer`). Switch on `Type()` before reading the fields.
- `YesNoAnswer.Value` is the probability of yes, not a severity: `0.5` means undecided, not "medium".
- A `YesNo` describes both answers or neither. Setting only one of `True` and `False` fails with `ErrInvalidQuestion` before anything is sent.
- `ScoreAnswer.Score` is a probability-weighted position across your levels, so it can land between them.
- `Confidence` is how concentrated the probabilities are. The package applies no threshold; that decision is yours.
- `Stats` reports the input tokens, the only ones providers bill, and how long the call took.

## `tools`

Handles the two chores of tool calling: writing parameter schemas and
dispatching the model's calls.

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

- `Tools` returns the tools sorted by name, so the tools section of the prompt is byte-identical across requests. Prompt caching needs that.
- `Tool(name)` returns one definition, or `false` if there is none. Its `Schema` map is shared with the registration; don't modify it.
- A `ToolBox` is safe for concurrent use. Tools can be added and removed while other goroutines list or run them.
- `SetInterceptor` installs a check that `Execute` runs after finding the tool and before calling the handler. If it returns an error, the handler never runs and `Execute` returns that error wrapped. It covers every tool, however it was registered (a pack, an MCP server, your own `Add`), so it is the one place to ask for approval, keep an audit trail, or refuse a command. `SetInterceptor(nil)` removes it; by default there is none.
- `ObjectBuilder` nests: pass one to `Object` or `ArrayOfObjects` for schemas of any depth.
- `Arguments` accessors return `ErrFieldNotFound` or `ErrInvalidFieldType` instead of panicking, and accept an `int` where JSON gives a `float64`. `Exists(key)` reports whether a field is present, whatever its type. For optional arguments, `GetOptionalString`, `GetOptionalInt`, `GetOptionalFloat64`, `GetOptionalBool` and `GetOptionalArrayOfStrings` take a fallback: a missing field returns it, and a present one is read like its `Get` counterpart, `ErrInvalidFieldType` included.
- `ValidToolName` and `SanitizeToolName` apply the providers' naming rules (up to 64 characters; letters, digits, `_`, `-`) to names from outside sources.

## `mcp`

Connects a stdio [MCP](https://modelcontextprotocol.io) server to a
`tools.ToolBox`, so its tools are called like any other. The official
[Go SDK](https://github.com/modelcontextprotocol/go-sdk) speaks the protocol;
this package owns the process, the namespacing and the `ToolBox` entries.

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

- Tools are registered as `"<Name>__<tool>"`, e.g. `playwright__browser_navigate`. A name the providers would reject is rewritten, not dropped; the server is still called by its own name.
- `NewClient` rejects a config without a `Name` (`ErrNameRequired`) or a `Command` (`ErrCommandRequired`) before launching anything.
- A server whose handshake declares no tools capability is never asked for tools. `RegisterTools` registers nothing and succeeds, so a server that only offers resources or prompts keeps running. A server that declares no capabilities at all is asked anyway.
- When a server sends `notifications/tools/list_changed`, its tools are registered again automatically: the list is fetched with a 30-second timeout and the `ToolBox` entries replaced. If the refresh fails, the old tools stay as they were. Calling `RegisterTools` again does the same by hand.
- `Close` stops the process, removes its tools, and aborts calls still waiting on the server. `Connected` reports whether the process is still up. A `Client` is safe for concurrent use.
- `Name` returns `ClientConfig.Name`, which also prefixes every tool the client registers.
- `Instructions` returns the usage instructions the server sent in its handshake, as an `*Instruction` labeled with the client's name, or nil if it sent none. They don't change for the client's lifetime. Put them in the system prompt when a server's guidance is worth keeping.
- `CallTool` calls one of the server's tools directly and returns its text result, on the same terms as a registered tool: `ToolCallTimeout`, progress resetting the timer, nil args sent as an empty object. It takes the server's own tool name and ignores `ExcludedTools`, so a program can use a tool without offering it to the model.
- `ExcludedTools` names server tools to leave unregistered, by the server's name for them, not the namespaced one. It applies every time the list is read, so an excluded tool stays out after a list change. `packs.CodingTools` uses it to leave out Serena's shell.
- `ToolCallTimeout` limits one tool call, 60 seconds by default. It is an idle timeout: every progress notification restarts it, so a tool that keeps reporting progress keeps running, and one that goes quiet fails that call. It runs inside the caller's context, so the caller's deadline survives and the agent loop reports the failure to the model and goes on. The handler sees the abort as `context.Canceled`, like any cancellation. `ErrRequestTimeout` is only recorded as the internal cancellation cause and never returned, so a caller can't yet tell a quiet server from a cancelled turn.
- Content the model can't read as text is summarized, not dropped. An image, audio clip, resource link or binary resource becomes a descriptor like `[image: image/png, 48213 bytes]`. Text passes through unchanged, and a result with only structured content is rendered as JSON.
- The server gets a filtered environment: `HOME`, `LOGNAME`, `PATH`, `SHELL`, `TERM`, `USER`, `TMPDIR`, `LANG`, `TZ`, `SSL_CERT_DIR`, `SSL_CERT_FILE`, the proxy variables in either case, and every `LC_` variable. Name anything else a server needs in `InheritEnv` to copy it from your process. The config holds names, never values. A name you haven't set is skipped, not passed empty.
- `Command` and `Args` run without a shell but are still trusted input. Take them from operator configuration, never from an untrusted source.

### `Manager`

Runs several MCP servers on demand against one `ToolBox`, for example to offer
a server's tools only while a user has it switched on.

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

`Register` stores a configuration without starting it. `Start` and `Stop` bring
a server up and down by name and keep its configuration. Starting a running
server does nothing, and a dead one is replaced. Both return
`ErrMCPNotRegistered` for an unknown name. `Status` shows which servers are
running, and `Close` stops them all but keeps the registrations. A `Manager` is
safe for concurrent use.

`Instructions` returns one `Instruction` per running server that sent
instructions, sorted by name, or nil if none did. Dead clients are discarded
along the way, as in `Status`.

## `skills`

A skill is a folder with a `SKILL.md`: frontmatter with a `name` and a
`description`, then the instructions. You add the folders a session should
have. Nothing is discovered automatically.

```go
collection := skills.NewCollection()
if err := collection.Add("./skills/git-release"); err != nil {
	log.Fatal(err)
}

if err := collection.AddClaudeSkill("git-release"); err != nil {
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

- Only names and descriptions reach the model up front, as an `<available-skills>` block appended to the system prompt. A skill's body loads when the model asks for it, so a long skill costs nothing until used.
- The session gets three tools: `skill_load` returns a skill's instructions and the list of files it ships, `skill_load_file` returns one of those files, and `skill_execute_file` runs one.
- `skill_load`, `skill_load_file` and `skill_execute_file` are reserved names. A tool already registered under one of them is replaced for the session and removed when it ends.
- `AddClaudeSkill` adds a skill by name from `~/.claude/skills` and otherwise works like `Add`. The name must be a single folder there. Anything that would leave the folder, `../other` included, fails with `ErrInvalidSkillName`, and a missing one gets `Add`'s `ErrSkillFolderNotFound`.
- An agent registers the collection in `StartSession`. Without an agent, `RegisterTools` adds the three tools to any `ToolBox` and `UnregisterTools` removes them. `Catalog` renders the `<available-skills>` block, and `Skills` lists the names, sorted.
- File access is confined to the skill folder with `os.OpenRoot`. A symlink that points outside is neither listed nor readable, and the model never learns the folder's real path.
- `skill_execute_file` runs the file directly from the skill's folder, with the model's arguments and no shell. The file needs its own execute bit and shebang; the package doesn't change modes or guess an interpreter from the extension. Only files the skill ships can run.
- A non-zero exit is a result: the tool returns the combined output and the exit status. It reports an error only when the process could not run at all.
- Output is collected up to 1 MiB, then the script is stopped and the result marked truncated. The exit status of a stopped script describes the kill.
- The script gets no stdin, so a read sees end of input at once.
- A script is stopped after two minutes, and the model is told it timed out; that's not an error. The two minutes apply only when the context has no deadline. A deadline you set is kept, and if it or a cancellation ends the script, the tool returns the context's error.
- Running a script leaves the `os.OpenRoot` sandbox: the process has your program's authority and inherits its environment, credentials included. Add only folders you trust, as with an `mcp` server command.
- `Add` reads the body and the file list once. Editing a skill on disk doesn't change a collection already built.
- Frontmatter is YAML, so `name` and `description` can be any YAML scalar: quoted, folded (`>`) or literal (`|`). Other keys are ignored, whatever they hold. Invalid YAML, or either key mapped to a non-scalar, fails with `ErrInvalidFrontmatter`.
- The catalog is sorted by name, so the system prompt is byte-identical across sessions built from the same collection, which prompt caching needs.

## `packs`

A pack is a bundle of ready-made tools. One call registers it in a `ToolBox`,
and the returned `ToolPack` removes it again.

```go
pack, err := packs.WebTools(ctx, toolBox)
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

`ToolPack.Instructions(ctx)` returns an `*mcp.Instruction` with the pack's
usage guidance for the model's system prompt, or nil if there is none, plus an
error. An MCP-served pack may ask its server under `ctx` and return the
server's error, so call it before `Close`; its guidance is the server's own.
The packs that serve their own tools return fixed text written in `packs`,
which adds to the tool descriptions and never fails.

### `WebTools`

`WebTools` gives the model web search, page fetching and site crawling through
[DonSeTch](https://github.com/dondai44423/donsetch). It needs no API key, only
the `donsetch` executable on `PATH`.

Worth knowing:

- The tools are `donsetch__web_search`, `donsetch__web_fetch` and `donsetch__web_crawl`.
- `ToolPack.Close` stops the server and removes its tools. Call it: nothing else owns the process, so a dropped `ToolPack` leaves the server running until the program exits.
- If registration fails, the server is stopped before `WebTools` returns.
- Tool calls time out after 15 minutes rather than the usual 60 seconds. `web_crawl` accepts a `deadline_s` of up to 600 and the other two a `deadline_ms` of up to 600000, so a shorter limit would cut a long call off before the server could report its own deadline. The server's error tells the model what to do next; a client-side timeout doesn't.
- The three tools carry about 15 KB of descriptions and schemas, paid on every request while registered. Close the pack when a session is done with the web.
- `packs.DonSeTchMCPConfig()` returns the `mcp.ClientConfig` the pack starts the server with, a new value each call. Change it freely and use it with `mcp.NewClient` and `RegisterTools` directly.
- `ToolPack.Instructions` returns whatever DonSeTch sent in its handshake, never an error. That's usually nil: DonSeTch puts its guidance in the tool descriptions.

### `CodingTools`

`CodingTools` gives the model a code base through
[Serena](https://github.com/oraios/serena): symbol-aware navigation and editing,
diagnostics, file access and read-only queries against other projects. It needs
no API key, only the `uvx` executable on `PATH`.

```go
pack, err := packs.CodingTools(ctx, toolBox, "ai-toolkit")
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

Worth knowing:

- The third argument is the project to activate at startup, as a name or a path, whichever `serena__activate_project` accepts. With an empty string, the model has to call `serena__activate_project` before it can reach any code.
- There is no shell. Serena's `execute_shell_command` is excluded through `ExcludedTools`. A model that builds or runs code also needs `ShellTools`, which is a separate pack, closed separately. To give Serena its shell back, take `packs.SerenaMCPConfig(project)`, clear `ExcludedTools`, and use `mcp.NewClient` directly.
- Serena's memory tools (`write_memory`, `read_memory`, `list_memories`, `edit_memory`, `delete_memory`, `rename_memory`) and the memory-backed `onboarding` are left out, so the model keeps no project memories. Serena starts in `no-memories` mode, which removes them on the server side and tells the model so. `ExcludedTools` lists them too, in case a later Serena changes what the mode covers.
- One project is active at a time, and activating another stops the previous one's language servers. `serena__query_project` reads a second project without switching, by running one read-only tool against a project Serena already knows; editing tools are refused there. Its symbolic tools need Serena's project server, a separate `serena start-project-server` process the pack doesn't launch. `read_file`, `list_dir`, `find_file` and `search_for_pattern` work without it. `serena__list_queryable_projects` names the projects that can be queried.
- The pack reads and writes files with your program's authority over the whole filesystem, and the model picks the project directory. Register it only for a model and conversation you'd trust with that, and remember that what the model reads in a repository can steer what it does next.
- Serena is launched from `git+https://github.com/oraios/serena`, unpinned, so each run gets whatever is on that branch. To pin it, point the `--from` argument of `packs.SerenaMCPConfig(project)` at a tag and use `mcp.NewClient` and `RegisterTools` directly, which is all this pack does.
- `ToolPack.Instructions` returns Serena's manual (about 8 KB), labeled `serena`, which explains how its tools fit together and when to prefer symbolic search over reading whole files. Each call fetches it through Serena's `initial_instructions` tool, because Serena's handshake instructions are a single line telling the model to make that call. Put the manual in the system prompt and the model needn't call `serena__initial_instructions`, though the tool stays registered. If the call fails (the server is gone, or `Close` ran), it falls back to the handshake line, or nil, and never returns an error.
- Tools are registered with a `serena__` prefix, so `find_symbol` becomes `serena__find_symbol`. The set is whatever Serena publishes minus the exclusions, so it changes with Serena.
- Tool calls time out after 360 seconds. Serena has its own per-call timeout, 240 seconds by default, and the client's sits above it so Serena's error reaches the model; a client-side timeout doesn't say what to do next.
- The first symbolic call on a newly activated project is slow: Serena downloads the language server if needed and indexes the project within that call.
- `ToolPack.Close` stops the server and removes its tools. Call it: nothing else owns the process, so a dropped `ToolPack` leaves the server running until the program exits.
- If registration fails, the server is stopped before `CodingTools` returns. If the server dies later, its tools are removed.
- The pack is much wider than `WebTools`: 23 tools and about 27 KB of descriptions and schemas, nearly double, paid on every request while registered. Close the pack when a session is done with the code.
- `packs.SerenaMCPConfig(project)` returns the `mcp.ClientConfig` the pack starts Serena with (the `desktop-app` context, the `query-projects` and `no-memories` modes, the exclusions above, and `--project` when the project is not empty), a new value each call, like `DonSeTchMCPConfig()`.

### `ThinkingTools`

`ThinkingTools` gives the model one tool, `thinking__sequentialthinking`, served
by the reference
[sequential-thinking](https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking)
MCP server. The model works a problem as numbered thoughts it can revise or
branch from. It needs no API key, only the `npx` executable on `PATH`.

```go
pack, err := packs.ThinkingTools(ctx, toolBox)
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

Worth knowing:

- The server keeps the thought history in memory, so it lasts as long as the pack. Close the pack and start a new one for a fresh history.
- The server is launched with `npx -y @modelcontextprotocol/server-sequential-thinking`, unpinned, so the first start downloads the package and later starts get whatever npm has cached or resolves as latest.
- `ToolPack.Close` stops the server and removes its tool. Call it: nothing else owns the process, so a dropped `ToolPack` leaves the server running until the program exits.
- If registration fails, the server is stopped before `ThinkingTools` returns. If the server dies later, its tool is removed.
- `packs.SequentialThinkingMCPConfig()` returns the `mcp.ClientConfig` the pack starts the server with, a new value each call. To pin a version, append `@<version>` to the package argument and use `mcp.NewClient` and `RegisterTools` directly.
- `ToolPack.Instructions` returns whatever the server sent in its handshake, never an error. Today that's nil: the server puts its guidance in the tool description.

### `ShellTools`

`ShellTools` gives the model one tool, `shell_run`, which runs a command line
with `/bin/sh`. Nothing is launched, so it takes no context:

```go
pack, err := packs.ShellTools(toolBox)
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

Worth knowing:

- The call gives a `command`, and optionally a `workdir` and a `timeout_ms`. It runs as `/bin/sh -c <command>`, in the program's working directory unless `workdir` says otherwise.
- `timeout_ms` goes from 1 to 600000, default 120000. A value outside that range fails with `ErrInvalidTimeout` before anything runs.
- A command that runs past its timeout is stopped, and the model is told to retry with a larger `timeout_ms`. That's a result, not an error, because an error alone wouldn't say what to do. Output collected up to that point is lost.
- The result has the exit status and the combined stdout and stderr, in the order written, in the same format as `skill_execute_file`. A non-zero exit is a result, not an error.
- Output is collected up to 1 MiB, then the command is stopped and the result marked truncated. The exit status of a stopped command describes the kill.
- The command gets no stdin, so a read sees end of input at once.
- The shell has your program's authority: the whole filesystem, the environment and its credentials. Register it only for a model and conversation you'd trust with a shell.
- `/bin/sh` is fixed and reads no startup file. `PATH` is the one your program inherited, so a directory added only in `.zshrc` or `.bashrc` isn't on it.
- `CodingTools` has no shell, so an agent that builds or runs code loads this pack too. The two are closed separately.
- If the `ToolBox` rejects the registration, `ShellTools` returns `ToolBox.Add`'s error and registers nothing.
- `ToolPack.Close` only removes the tool. There is no process, so a dropped `ToolPack` just leaves the tool registered.
- The tool carries about 700 bytes of description and schema, paid on every request while registered.
- `ToolPack.Instructions` returns guidance labeled `shell`: use it for terminal work, read and change files with the file tools rather than commands, and take commands from the user or the repository's own configuration instead of inventing them.

### `FileTools`

`FileTools` gives the model the files under one folder it can't leave. It suits
an agent that writes reports or notes rather than code, and has no business
loading `CodingTools`:

```go
pack, err := packs.FileTools(toolBox, "./workspace")
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

| Tool | What it does |
| --- | --- |
| `file_read` | Reads a text file a page at a time: `path`, and optionally `offset` and `limit` |
| `file_write` | Writes a whole file, creating the folders its path needs |
| `file_edit` | Replaces one piece of text in a file |
| `file_list` | Lists one folder, sorted by name, with each entry's full path |
| `file_search` | Finds the lines matching a regular expression across a folder's files |
| `file_delete` | Removes a file, or a folder that is already empty |
| `file_workdir` | Returns the root's absolute path, for naming a file to a tool outside the root |

Worth knowing:

- The confinement is `os.Root`. Paths are relative to the root, and a path that leaves it (climbing out, absolute, or through a symbolic link) is refused. No other pack has a boundary: `CodingTools` and `ShellTools` have the program's full authority.
- `FileTools` fails, registering nothing, when the root can't be opened. The folder must exist; the pack doesn't create it.
- `file_read` returns `<file lines="1-40 of 120">`, so the model can tell a page from a whole file and ask for the next `offset`. By default it reads at most 2000 lines, and never more than 1 MiB.
- Arguments are always relative to the root. An absolute path is refused, even one inside the root. Two results do include absolute paths, for passing to tools outside the root: `file_write` answers `wrote 8 bytes to notes.md - /Users/you/workspace/notes.md`, and `file_list` returns entries like `<file name="q1.md" size="8" path="/Users/you/workspace/reports/q1.md"/>` and `<dir name="2026" path="/Users/you/workspace/reports/2026"/>`. Names and paths are quoted Go-style, so a `"` in a name comes out as `\"`.
- `file_edit` writes nothing unless `old_string` appears exactly once. No match is `ErrNoMatch`, several is `ErrManyMatches`, and either way the file is untouched.
- `file_search` takes a Go regular expression in `pattern`, and optionally `path` (which folder), `glob` (which file names), `recursive` (default `true`) and `limit` (default 100 matches). It answers `<search matches="3" files="2">` around one `<match path="notes/q3.md" line="12">…</match>` per line. These paths are relative to the root, unlike `file_list`'s, so a match goes straight into `file_read`.
- `glob` is matched against the file's name only, so `*.md` finds one at any depth; `path` narrows the search to a subtree. Files with a NUL byte in the first 8 KiB are skipped as binary, as are unreadable files and files with a line over 1 MiB.
- A result cut short at `limit` has `truncated="true"` on `<search>`. The search looks one match past the limit, so a search that ends exactly on it is reported complete. A `<match>` has the same attribute when its line was over 500 bytes and got cut at the last whole character that fits, so one long line can't crowd out the rest. The two are independent. `limit` has no maximum.
- A `pattern` or `glob` that doesn't compile fails with `ErrInvalidPattern` before anything is read. A glob whose fault only shows against a particular file name fails when the search reaches such a name. A `limit` below one is `ErrInvalidRange`.
- `file_delete` won't delete a folder that has anything in it, so no single call deletes recursively. Delete a tree file by file.
- The root isn't secret. `file_workdir` reports it, `file_write` and `file_list` include it, and error messages quote full paths. That's what lets a file written here be passed to `shell_run` or a `CodingTools` tool. Use a folder whose path is safe to disclose.
- `file_workdir` takes no arguments and returns the root as an absolute path, fixed when the pack was built. A relative root still comes back absolute, and a later `chdir` doesn't change it.
- `ToolPack.Close` removes the seven tools and closes the root.
- The seven tools carry about 4.1 KB of descriptions and schemas, paid on every request while registered. `file_search` is the largest, at about 1.1 KB.
- `ToolPack.Instructions` returns guidance labeled `file`: the tools reach only the root, named by its absolute path, and paths are relative to it; use `file_search` rather than reading everything; give `file_edit` enough context to be unambiguous; and never delete work in bulk.

### `DateTools`

`DateTools` tells the model when it is. A model has no clock, and the date it
remembers is from its training, so any date it writes without asking is a
guess:

```go
pack, err := packs.DateTools(toolBox)
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

Worth knowing:

- Three tools, no arguments. `current_date` returns `2026-09-20`, `current_time` returns `15:04:05.000` (24-hour), and `time_zone` returns `WEST (UTC+01:00)`.
- `current_time` has no date or zone, and `current_date` no time. A full timestamp takes all three calls. The split keeps the common question, today's date, to one call that can't be mistaken for a moment in time.
- `time_zone` reports the host's zone from `time.Now().Zone()`: the abbreviation and the UTC offset. A zone without an abbreviation gives just the offset, e.g. `UTC+00:45`. The offset is the current one, so during daylight saving it's the summer offset.
- All three use the host clock in the host's zone. There's no way to ask for another zone or set the clock.
- Register the pack wherever a date ends up in the answer: a report header, a filing period, a valuation date. Without it, the model takes the date from its training and states it as fact.
- `ToolPack.Close` removes the three tools. There is no process, so a dropped `ToolPack` just leaves them registered.
- The three tools carry about 700 bytes of descriptions and schemas, paid on every request while registered.
- `ToolPack.Instructions` returns guidance labeled `date`: call the tools instead of trusting training data wherever the answer has a date or deadline, and call them again in a long session.

### `ClassifyTools`

`ClassifyTools` lets the model hand a judgement call to a `classify` model and
get calibrated probabilities back. You build the client; the pack only
registers the tools:

```go
client, err := classify.New(classify.Config{
	Provider: classify.ProviderOpenRouter,
	APIKey:   os.Getenv("OPENROUTER_API_KEY"),
	Model:    "typesafe/jev-1.13",
})
if err != nil {
	log.Fatal(err)
}

pack, err := packs.ClassifyTools(toolBox, client)
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

| Tool | Arguments | Returns |
| --- | --- | --- |
| `classify_yes_no` | `input`, `instructions`, and `true` and `false`, what each answer means | `{"yes_probability": 0.87}` |
| `classify_choice` | `input`, `instructions`, `options`, each a `name` and an optional `description` | `{"selected": "payments", "probabilities": {"payments": 0.81, "frontend": 0.19}, "confidence": 0.62}` |
| `classify_score` | `input`, `instructions`, `levels`, lowest to highest | `{"score": 1.4, "probabilities": [{"level": "Next release", "probability": 0.1}, …], "confidence": 0.3}` |

Worth knowing:

- One question per call. Several questions about the same input take several calls, each billed on the whole `input`.
- Everything the model puts in `input` goes to OpenRouter. Register the pack only where that's acceptable.
- The classification model sees the `input` and the question, nothing of the conversation, so the tool descriptions tell the model to make the input self-contained.
- `yes_probability` is the probability of yes, so `0.5` means undecided. `score` is a probability-weighted position from `0` to the last level's index and can fall between levels. `confidence` is how concentrated the probabilities are, not how likely the answer is to be right.
- A `classify_choice` with fewer than two options or a repeated option, and a `classify_score` with fewer than two levels, fail with `ErrInvalidQuestion` before anything is sent. The provider enforces its own limits (at most 10 levels, 255 options), and its error reaches the model like any tool error.
- The pack adds no timeout or retry. Calls run under the caller's context, and the `classify` client already retries 429 and 5xx responses.
- You own the client. `ToolPack.Close` only removes the three tools.
- The three tools carry about 2.4 KB of descriptions and schemas, paid on every request while registered.
- `ToolPack.Instructions` returns guidance labeled `classify`: use the tools when a labeled answer settles a judgement call, read the probabilities as calibrated, make the input self-contained, and treat the answer as advice, saying when you went against it.

## `agent`

Ties `llm` and `tools` into a loop: send the user's input, run the tools the
model asks for, feed back the results, and repeat until the model gives a final
answer. You don't write that loop yourself.

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

- A failing tool is reported to the model as its error text, so the model can recover and the turn goes on.
- The tool list is read from the `ToolBox` once per `Process` call and stays fixed for that round. A tool registered during the round, by an MCP server changing its list, say, is offered from the next `Process`. A tool removed during the round is still offered until then, and calling it fails with `ErrToolNotFound`, which the model sees as tool error text.
- Once a completed turn goes past `Config.CompactionThresholdPercent` of the context window (85% by default), the older turns are summarized into one message. The system prompt and recent turns are kept word for word.
- `Config.MaxIterations` caps the model/tool rounds per `Process` call. Zero means no limit; reaching the cap returns `ErrMaxIterations`.
- `Response.Metadata` reports token usage, stop reason, timing per phase, and the number of rounds and tool calls.
- `StartSession` sets everything the model sees: the system prompt, the `ToolBox` it may call, and the `skills.Collection` it may load from. They last until `Close` or the next `StartSession`, so one agent can run differently equipped sessions.
- The tools of a `SessionConfig.Skills` collection are registered in the session's `ToolBox`, and its catalog is appended to the prompt. `Close` removes those tools.
- `Messages` returns a copy of the conversation as the model sees it: the system message (the prompt plus any skill catalog), then every turn, tool calls and results included. Turns folded by compaction show only as their summary. It returns nil when there is no session.
- `SessionConfig.Messages` resumes a saved conversation. The messages follow the new system message, and any `llm.SystemMessage` among them is skipped, so the new prompt applies. Storing the messages in between is up to you. Provider-specific assistant data, such as Anthropic thinking blocks, doesn't survive the round trip; each assistant turn is sent as its text and tool calls.
- `SessionID` identifies the session. `StartSession` and `ResetSession` each give it a new id, and it is `""` when there is no session. `SessionConfig.ID` resumes a conversation under a known id, used as given.
- Install a `Feedback` with `SetFeedback` to follow tool calls and session events; the default prints nothing. `ToolCalled(toolName, args)` fires just before a tool runs, with the model's arguments: JSON types, so numbers are `float64`, and nil when there are none. The tool runs with that map, so read it but don't modify it. `ToolReturned(toolName, result, err, elapsed)` fires just after, with the result (empty when `err` is set) and the time taken. Tools run one at a time, so each `ToolReturned` follows its `ToolCalled`. `InterimTextReceived(content)` fires when a response that asks for tools also has text, typically the model saying what it's about to do, before that response's `TokensUsed`. It never fires for empty text or the final answer, which is in `Response.Content`. `TokensUsed(totalTokens)` fires after each response that asks for tools, so usage can be tracked before the final answer, whose usage is in `Response.Metadata`. `StartSession` fires `SessionStarted()` for a new conversation, or `SessionResumed(sessionID)` instead when `SessionConfig.Messages` restores at least one turn.

## License

MIT, see [LICENSE](LICENSE).
