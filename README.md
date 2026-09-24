# ai-toolkit

[![Go Reference](https://pkg.go.dev/badge/github.com/jjmrocha/ai-toolkit.svg)](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit)

A personal, highly opinionated set of Go packages for working with chat-based
LLMs. It's built for my own use and reflects my own taste in API design — there
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
| [`llm`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/llm) | One chat API across three providers | — |
| [`classify`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/classify) | Typed questions answered with probabilities, not text | — |
| [`tools`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/tools) | Registers tools and dispatches the model's calls | `llm` |
| [`mcp`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/mcp) | Turns an MCP server's tools into `tools` entries | `llm`, `tools` |
| [`skills`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/skills) | On-demand instructions the model loads by name | `llm`, `tools` |
| [`agent`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/agent) | Runs the call-tool-feed-back loop for you | `llm`, `tools`, `skills` |
| [`packs`](https://pkg.go.dev/github.com/jjmrocha/ai-toolkit/packs) | Ready-made tool bundles, registered in one call | `classify`, `mcp`, `tools` |

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
- `Config.Effort` maps one knob, `EffortOff` through `EffortMax`, onto Anthropic's adaptive-thinking effort level and OpenRouter/Ollama's reasoning level. The values are relative rungs, not provider literals, so the same `Effort` reaches each backend as whatever that backend calls it.
- `Config.Models` lists what `ChangeModel` may switch to mid-conversation; the active model is always included.
- `ChangeModel` and `ChangeEffort` both validate before they mutate and return an error otherwise, so a rejected switch leaves the client on its current settings.

## `classify`

Asks a classification model typed questions about an input and gets typed answers back.
No prose to parse: each answer carries the probabilities behind it. The provider
is OpenRouter, which serves TypeSafe's Jev models.

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

- `Request.Questions` and `Response.Answers` share the identifiers you choose. They are never sent to the model.
- Every question in a request is answered in parallel and in isolation: none of them sees another's answer.
- `Answer` is a sealed interface — `YesNoAnswer`, `ChoiceAnswer`, `ScoreAnswer` — so switch on `Type()` before reading an answer's fields.
- `YesNoAnswer.Value` is the probability of yes, not a severity: `0.5` means undecided, not "medium".
- `ScoreAnswer.Score` is a probability-weighted position across your levels, so it lands between them.
- `Confidence` describes how concentrated the probabilities are. What to do below a threshold is yours to decide; the package never decides for you.
- `Stats` reports the input tokens — the only ones providers bill — and how long the call took.

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
- `Tool(name)` looks one definition up by name, returning `false` when nothing is registered under it — a cheaper check than scanning `Tools`. The value it hands back is a shallow copy, so its `Schema` map must be left alone.
- A `ToolBox` is safe for concurrent use: tools can be added and removed while other goroutines list or execute them.
- `SetInterceptor` installs a gate `Execute` consults after it finds the tool and before it runs the handler: return an error and the call is blocked, the handler never runs, and the error comes back wrapped. It covers every tool in the box however it was registered — by a pack, by an MCP server at runtime, or by your own `Add` — so it is the one place to ask for approval, keep an audit trail, or refuse a command outright. `SetInterceptor(nil)` clears it; unguarded is the default.
- `ObjectBuilder` nests — pass one to `Object` or `ArrayOfObjects` to describe schemas of any depth.
- `Arguments` accessors return `ErrFieldNotFound` or `ErrInvalidFieldType` instead of panicking, and take an `int` where JSON handed you a `float64`. `Exists(key) bool` reports whether a field is there at all, whatever its type — which is how an optional argument gets a default, read only when the call set it, rather than asking for it and sorting `ErrFieldNotFound` out of the error that comes back.
- `ValidToolName` and `SanitizeToolName` apply the providers' naming rules (64 characters; letters, digits, `_`, `-`) to names from outside sources.

## `mcp`

Connects a stdio-based [MCP](https://modelcontextprotocol.io) server to a
`tools.ToolBox`, so the tools it exposes become callable like any other tool. The
protocol is spoken by the official
[Go SDK](https://github.com/modelcontextprotocol/go-sdk); this package owns the
process, the namespacing, and the mapping onto `ToolBox`.

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
- `NewClient` rejects a config without a `Name` (`ErrNameRequired`) or without a `Command` (`ErrCommandRequired`) before launching anything.
- A server whose handshake declares no tools capability is never asked for a tool list. `RegisterTools` registers nothing and succeeds, so a resources-only or prompts-only server keeps running instead of being torn down for declining a method it never claimed. A server that declares no capabilities at all is asked anyway.
- A server that announces `notifications/tools/list_changed` has its tools registered again automatically: the list is fetched afresh under its own thirty-second timeout and the `ToolBox` entries are replaced. A refresh that fails changes nothing — the tools already registered stay exactly as they were. Calling `RegisterTools` again does the same by hand, so a caller that knows the list has moved need not tear the client down.
- `Close` shuts the process down and removes the tools it registered, aborting any call still waiting on the server. `Connected` reports whether the process is still up. A `Client` is safe for concurrent use.
- `ExcludedTools` names tools the server publishes that you do not want registered, by the name the server publishes them under rather than the namespaced one. The filter applies every time the list is read, so a tool named here stays unregistered when the server announces a change to its list, and a name the server never publishes is ignored. It is how you drop one capability from a server you otherwise want — `packs.CodingTools` uses it to leave Serena's shell out.
- `ToolCallTimeout` bounds one call to this server's tools, defaulting to sixty seconds. It is an idle timeout rather than a total budget: every progress notification the server sends restarts the clock, so a tool that reports progress runs as long as it keeps reporting, while one that goes quiet fails that single call. The timeout sits inside the caller's own context, so a failed call leaves the caller's deadline intact — the agent loop reports the failure to the model and carries on rather than losing the turn. The abort reaches the handler as `context.Canceled`, the same as a cancellation from above: `ErrRequestTimeout` is recorded as the internal context's cancellation cause and is never returned, so a caller cannot currently tell a quiet server from a cancelled turn.
- Content the model cannot read as text is summarised rather than dropped. An image, audio clip, resource link, or binary embedded resource becomes a short descriptor such as `[image: image/png, 48213 bytes]`; text content and text-bearing resources pass through unchanged, and a result carrying only structured content is rendered as its JSON.
- The server gets a filtered environment, not yours: `HOME`, `LOGNAME`, `PATH`, `SHELL`, `TERM`, `USER`, `TMPDIR`, `LANG`, `TZ`, `SSL_CERT_DIR`, `SSL_CERT_FILE`, the proxy variables in both cases, and every `LC_` variable. Name anything else a particular server needs in `InheritEnv` and it is copied from the calling process — the config names variables, it never holds their values. A name you did not set is skipped rather than passed on empty.
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
restart; a server whose process has died is replaced on the next `Start`, and
starting one that is already running is a no-op. Either call returns
`ErrMCPNotRegistered` for a name that was never registered. `Status` reports
which are running and `Close` stops everything it started, keeping the
registrations so the same `Manager` can bring them back up. Safe for concurrent
use.

## `skills`

A skill is a folder with a `SKILL.md` inside: frontmatter carrying a `name` and a
`description`, and a body holding the instructions. You add the folders a session
should have — nothing is discovered automatically.

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

- Only names and descriptions reach the model up front, as an `<available-skills>` block appended to the session's system prompt. Bodies load on demand, so a long skill costs nothing until it is used.
- Three tools are registered for the session: `skill_load` returns a skill's instructions plus the list of files it ships, `skill_load_file` returns one of those files, and `skill_execute_file` runs one of them.
- **`skill_load`, `skill_load_file` and `skill_execute_file` are reserved tool names.** A tool already registered under any of them is replaced while the session lasts, and removed when it ends.
- `AddClaudeSkill` adds a skill by name from the user's Claude skills folder, `~/.claude/skills`, and is `Add` in every other respect. The name has to be a single folder in there — anything that would step outside it, `../other` included, is rejected with `ErrInvalidSkillName`, and a name that is not there gets `Add`'s own `ErrSkillFolderNotFound`.
- An agent wires the collection up on `StartSession`; on its own, `RegisterTools` adds the three tools to any `ToolBox` and `UnregisterTools` takes them back out. `Catalog` renders the `<available-skills>` block, and `Skills` lists the names added so far, sorted.
- File access is confined to the skill folder with `os.OpenRoot`, so a symlink pointing outside it is neither listed nor readable, and the model is never told the folder's real path.
- `skill_execute_file` runs the file directly, from the skill's folder, with the arguments the model supplies and no shell. The file needs its own execute bit and shebang; the package never changes file modes, and it infers no interpreter from the extension. A file the skill does not ship cannot be run.
- A non-zero exit is a result, not a failure: the tool returns the process's combined output and its exit status, and reports an error only when the process could not run at all.
- Output is collected up to 1 MiB, after which the script is stopped and the result is marked truncated. A stopped script's exit status describes the kill rather than a choice it made.
- The script gets no stdin, so one that reads input sees end of input at once instead of waiting.
- A script is stopped after two minutes, and the model gets a result saying it timed out rather than an error. That limit is a default for a context with no deadline: a deadline the caller already set is kept whatever its length, and when it or a cancellation ends the script, the tool reports the context's error. Execution also leaves the `os.OpenRoot` sandbox behind: the process runs with the same authority as the program that started it and inherits its environment, credentials included, so add only folders you trust, exactly as with an `mcp` server command.
- The body and the file list are read once, by `Add`. Editing a skill on disk does not change a collection already built.
- Frontmatter is parsed as YAML, so any valid YAML scalar works for `name` and `description` — quoted, folded (`>`) or literal (`|`). Keys other than those two are ignored whatever they hold, including nested mappings and sequences. Content that is not valid YAML, or that maps either key to something other than a scalar, is rejected with `ErrInvalidFrontmatter`.
- The catalog is sorted by name, so the system prompt stays byte-identical across sessions built from the same collection — which is what prompt caching needs.

## `packs`

A pack is a bundle of tools that arrives ready to use: one call registers it in
a `ToolBox`, and the returned `ToolPack` takes it back out again.

```go
pack, err := packs.WebTools(ctx, toolBox)
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

### `WebTools`

`WebTools` gives the model web search, page fetching and site crawling, backed by
[DonSeTch](https://github.com/dondai44423/donsetch). It is keyless, so the only
prerequisite is the `donsetch` executable on `PATH`.

Worth knowing:

- The server publishes `web_search`, `web_fetch` and `web_crawl`, registered as `donsetch__web_search`, `donsetch__web_fetch` and `donsetch__web_crawl`.
- `ToolPack.Close` stops the server process and removes its tools from the `ToolBox`. It must be called: nothing else owns the process, so a dropped `ToolPack` leaves the server running for the life of the program.
- A registration that fails closes the server before returning, so a failed `WebTools` leaves nothing behind.
- The tool call ceiling is 15 minutes rather than the two-minute default. `web_crawl` accepts a `deadline_s` of up to 600 seconds and the other two a `deadline_ms` of up to 600000, so a shorter ceiling would kill a long call before the server could report its own deadline — and the server's error tells the model what to do next, where a client-side timeout does not.
- The three tools carry roughly 15 KB of descriptions and schemas, which every request pays for while they are registered. Close the pack when a session has finished with the web.
- `packs.DonSeTchMCPConfig()` returns the `mcp.ClientConfig` this pack starts the server from, a fresh value each call that shares nothing with the pack. Adjust the returned config freely — a variant built from it goes through `mcp.NewClient` and `RegisterTools`, not through `WebTools`.

### `CodingTools`

`CodingTools` gives the model a code base: symbol-aware navigation and editing,
diagnostics, file and directory access, project memories and read-only queries
against other projects, backed by
[Serena](https://github.com/oraios/serena). It is keyless, so the only
prerequisite is the `uvx` executable on `PATH`.

```go
pack, err := packs.CodingTools(ctx, toolBox)
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

Worth knowing:

- The server starts with no project. The model reaches a code base by calling `serena__activate_project`, and the symbolic tools fail until it does.
- The pack carries no shell: Serena's `execute_shell_command` is left out through `ExcludedTools`, and nothing takes its place. A model that has to build or run what it wrote needs `ShellTools` on the same `ToolBox`, which registers `shell_run` under a pack of its own, closed separately. To give Serena its own shell back instead, take `packs.SerenaMCPConfig()`, clear its `ExcludedTools`, and go through `mcp.NewClient` directly.
- One project is active at a time, and activating another shuts the previous one's language servers down. A second code base is read without switching through `serena__query_project`, which runs one read-only tool against a project Serena already has registered — the editing tools are refused there, so a queried repository cannot be changed. Its symbolic tools reach the other project through Serena's project server, which is a separate `serena start-project-server` process the pack does not launch; `read_file`, `list_dir`, `find_file` and `search_for_pattern` need no such thing. `serena__list_queryable_projects` names what can be queried, and a repository Serena has never registered is not on that list.
- This pack reads and writes files with the authority of the program that started it — the whole filesystem — and the model, not the caller, picks the project directory. Register it only for a model and a conversation you would trust with that reach, and remember that anything the model reads out of a repository can steer what it does next.
- The pack launches Serena from `git+https://github.com/oraios/serena`, unpinned, so a run executes whatever is on that branch at the time. Pinning is the operator's to add: take `packs.SerenaMCPConfig()`, point its `--from` argument at a tag, and use `mcp.NewClient` with `RegisterTools` directly, which is all this pack does.
- Serena's own manual — how its tools fit together, and when to prefer symbolic search over reading whole files — is a tool call away as `serena__initial_instructions`. It is worth having the model read it early, because the tool descriptions alone do not convey the workflow.
- Serena's tools are registered under a `serena__` prefix, so `find_symbol` becomes `serena__find_symbol`. The exact set is whatever the server publishes minus the exclusions, so it moves with Serena's own development rather than being fixed here.
- The tool call ceiling is 360 seconds rather than the two-minute default. Serena enforces its own per-call timeout, 240 seconds by default, and the client ceiling sits above it so the server's error reaches the model — a client-side timeout does not say what to do next.
- The first symbolic call on a newly activated project is the slow one: Serena downloads that language's server if it is missing and indexes the project inside that call's budget.
- `ToolPack.Close` stops the server process and removes its tools from the `ToolBox`. It must be called: nothing else owns the process, so a dropped `ToolPack` leaves the server running for the life of the program.
- A registration that fails closes the server before returning, so a failed `CodingTools` leaves nothing behind. A server that later dies on its own drops its own tools.
- This is a far wider pack than `WebTools`: 30 tools carrying roughly 30 KB of descriptions and schemas, twice the web pack's bill and paid on every request while they are registered. Close the pack when a session has finished with the code.
- `packs.SerenaMCPConfig()` returns the `mcp.ClientConfig` this pack starts the server from — Serena's `desktop-app` context with its `query-projects` mode added and `execute_shell_command` in `ExcludedTools` — on the same terms as `DonSeTchMCPConfig()`: a fresh value each call, free to adjust and hand to `mcp.NewClient`.

### `ShellTools`

`ShellTools` gives the model one tool, `shell_run`, that runs a command line
with `/bin/sh`. Nothing is launched to serve it, so it takes no context:

```go
pack, err := packs.ShellTools(toolBox)
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

Worth knowing:

- The call supplies the `command`, and optionally a `workdir` and a `timeout_ms`. The command runs as `/bin/sh -c <command>` from the program's own working directory unless `workdir` says otherwise.
- `timeout_ms` runs from 1 to 600000 and defaults to 120000. A value outside that range is rejected with `ErrInvalidTimeout` before anything runs.
- A command that outlasts its timeout is stopped, and the model is told to retry with a larger `timeout_ms` — a result rather than an error, because the error text alone would not say what to do next. The output collected up to that point is lost.
- The result carries the exit status and the combined stdout and stderr, in the order the command wrote them, in the same shape `skill_execute_file` uses. A non-zero exit is a result, not an error.
- Output is collected up to 1 MiB, after which the command is stopped and the result is marked truncated. A stopped command's exit status describes the kill rather than a choice it made.
- The command gets no stdin, so one that reads input sees end of input at once instead of waiting.
- The shell runs with the authority of the program that registered the tool: the whole filesystem, the environment and its credentials. Register it only for a model and a conversation you would trust with a shell.
- `/bin/sh` is fixed, and no startup file is read. `PATH` is the one the program itself inherited, so a directory added only in an interactive shell's `.zshrc` or `.bashrc` is not on it.
- `CodingTools` carries no shell, so an agent that has to build or run what it wrote loads this pack alongside it. The two are closed separately.
- `ShellTools` fails, registering nothing, when the `ToolBox` rejects the registration — it passes `ToolBox.Add`'s error through rather than swallowing it.
- `ToolPack.Close` only removes the tool from the `ToolBox`. There is no process to leak, so a dropped `ToolPack` costs nothing beyond the tool staying registered.
- The tool carries roughly 700 bytes of description and schema, which every request pays for while it is registered.

### `FileTools`

`FileTools` gives the model files under one folder it cannot leave — for an
agent that writes reports or notes rather than code, and so has no business
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
| `file_write` | Writes a file whole, creating the folders its path needs |
| `file_edit` | Replaces one piece of text inside a file |
| `file_list` | Lists one folder, sorted by name, with each entry's full path |
| `file_search` | Finds the lines matching a regular expression across a folder's files |
| `file_delete` | Removes a file, or a folder that is already empty |
| `file_workdir` | Returns the root's absolute path, for naming a file to a tool outside the root |

Worth knowing:

- The confinement is `os.Root`. Paths are relative to the root, and one that leaves it — by climbing out, by being absolute, or through a symbolic link — is refused rather than followed. This is the one pack with a boundary: `CodingTools` and `ShellTools` both run with the program's full authority.
- `FileTools` fails, registering nothing, when the root cannot be opened. The folder has to exist; the pack does not create it.
- `file_read` returns `<file lines="1-40 of 120">`, so the model can tell a page from a whole file and call again with a larger `offset`. It reads at most 2000 lines by default and stops at 1 MiB, whichever comes first.
- Arguments are always relative to the root; an absolute path is refused, even one that points inside it. Two results hand out absolute paths anyway, for the model to pass on to a tool that is not confined here: `file_write` answers `wrote 8 bytes to notes.md - /Users/you/workspace/notes.md`, and `file_list` gives one element per entry — `<file name="q1.md" size="8" path="/Users/you/workspace/reports/q1.md"/>` and `<dir name="2026" path="/Users/you/workspace/reports/2026"/>`.
- `file_edit` writes nothing unless its `old_string` appears exactly once — zero matches is `ErrNoMatch`, several is `ErrManyMatches`. An edit never lands somewhere the model did not mean, and the file is left untouched on either error.
- `file_search` takes a Go regular expression in `pattern`, and optionally `path` (which folder), `glob` (which file names), `recursive` (default `true`) and `limit` (default 100 matches). It answers `<search matches="3" files="2">` wrapping one `<match path="notes/q3.md" line="12">…</match>` per line. Unlike `file_list`, the paths here are **relative** to the root — the form every other file tool takes, so a match goes straight back into `file_read`.
- `file_search` matches `glob` against the file's name alone, never its path, so `*.md` finds one at any depth; `path` is what narrows the search to a subtree. A file holding a NUL byte in its first 8 KiB is passed over as binary, as are unreadable files and any file with a line over 1 MiB.
- `file_search` marks a result it cut short with `truncated="true"` on the `<search>` element, and it means there is genuinely more: it collects one match past `limit`, so a search that ends exactly on the limit is reported as complete. A single `<match>` carries the same attribute when the line itself ran long: a matching line over 500 bytes is cut at the last whole character that fits, so one very long line cannot crowd out the rest of the result. The two are independent — a complete search can hold cut matches, and a cut search can hold whole ones. There is no ceiling on `limit`, so a large one is honored in full. A pattern or glob that does not compile is `ErrInvalidPattern`, and nothing is read; a `limit` below one is `ErrInvalidRange`.
- `file_delete` will not empty a folder: a folder that still holds anything is kept, so nothing recursive happens behind one call. Deleting a tree means deleting its files first.
- The root is not a secret. `file_workdir` reports it, `file_write` and `file_list` embed it, and error text quotes the failing path in full — which is what lets a file written here be named to `shell_run` or a `CodingTools` tool. Root the pack at a folder whose path is safe to disclose.
- `file_workdir` takes no arguments and reports the root as an absolute path, resolved when the pack was built. A pack rooted at a relative path still reports an absolute one, and a later `chdir` does not change the answer.
- `ToolPack.Close` removes the seven tools and closes the root. There is no process to leak.
- The seven tools carry roughly 4.1 KB of descriptions and schemas, which every request pays for while they are registered. `file_search` is the largest single tool of the seven, at about 1.1 KB.

### `DateTools`

`DateTools` tells the model when it is. A model has no clock, and the date it
remembers is the one it was trained on, so anything it dates without asking is a
guess:

```go
pack, err := packs.DateTools(toolBox)
if err != nil {
	log.Fatal(err)
}

defer pack.Close()
```

Worth knowing:

- Three tools, none of which takes an argument. `current_date` returns `2026-09-20`, `current_time` returns `15:04:05.000` on a 24-hour clock, and `time_zone` returns `WEST (UTC+01:00)`.
- `current_time` carries no date and no zone, and `current_date` no time. A model that needs a full timestamp calls all three; the split keeps the common case — what is today's date — a one-call answer that cannot be misread as a moment in time.
- `time_zone` reports the host's zone from `time.Now().Zone()`: the abbreviation and the offset from UTC. A zone with no abbreviation gives the offset alone, `UTC+00:45`. The offset is the one in force now, so a zone that observes daylight saving reports the current side of it, not the standard one.
- All three read the host clock in the host's own zone. There is no argument for a zone to convert to and no way to set the clock.
- Register the pack wherever a date reaches the answer — a report header, a filing period, a valuation's as-of date. Without it a model fills those from training data and states the result as fact.
- `ToolPack.Close` removes the three tools. Nothing is launched to serve them, so a dropped `ToolPack` costs nothing beyond the tools staying registered.
- The three tools carry roughly 700 bytes of descriptions and schemas, which every request pays for while they are registered.

### `ClassifyTools`

`ClassifyTools` lets the model hand a judgement call to a `classify` model
instead of making it itself, and get calibrated probabilities back. You build
the client; the pack only registers the tools:

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
| `classify_yes_no` | `input`, `instructions`; optionally `true` and `false`, what each answer means | `{"yes_probability": 0.87}` |
| `classify_choice` | `input`, `instructions`, `options` — each a `name` and an optional `description` | `{"selected": "payments", "probabilities": {"payments": 0.81, "frontend": 0.19}, "confidence": 0.62}` |
| `classify_score` | `input`, `instructions`, `levels`, lowest to highest | `{"score": 1.4, "probabilities": [{"level": "Next release", "probability": 0.1}, …], "confidence": 0.3}` |

Worth knowing:

- One question per call. Several questions about the same input are several calls, each billed on its input tokens — the whole `input` included.
- Everything the model puts in `input` is sent to OpenRouter. Register the pack only where that is acceptable.
- The classification model sees the `input` and the question, nothing of the conversation, so the tool descriptions tell the model to make the input self-contained.
- `yes_probability` is the probability of yes, so `0.5` means undecided. `score` is a probability-weighted position from `0` to the last level's index and can fall between levels. `confidence` is how concentrated the probabilities are, not how likely the answer is to be right.
- A `classify_choice` with fewer than two options or the same option twice, and a `classify_score` with fewer than two levels, fail with `ErrInvalidQuestion` before anything is sent. The provider's own limits — at most 10 levels, 255 options — are left to it, and its error reaches the model like any other tool error.
- There is no timeout or retry in the pack: the call runs under the caller's context, and the `classify` client already retries 429 and 5xx responses.
- The caller owns the client. `ToolPack.Close` removes the three tools and nothing else.
- The three tools carry roughly 2.4 KB of descriptions and schemas, which every request pays for while they are registered.

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
- The tool list is read from the `ToolBox` once per `Process` call and stays fixed for that round, so the menu never shifts under the model mid-round. A tool registered while the round runs — by an MCP server announcing a tool-list change, say — is offered from the next `Process` on; one removed the same way stays on offer until then and fails with `ErrToolNotFound` if called, which the model sees as tool-error text.
- Once a completed turn crosses `Config.CompactionThresholdPercent` of the model's context window (85% by default), the older turns are summarized into a single message while the system prompt and recent turns are kept verbatim.
- `Config.MaxIterations` caps the model/tool rounds per `Process` call; zero means no limit, and hitting the cap returns `ErrMaxIterations`.
- `Response.Metadata` reports token usage, stop reason, per-phase timing, and iteration and tool-call counts.
- `StartSession` declares everything the model sees: the system prompt, the `ToolBox` it may call, and the `skills.Collection` it may load from. All three last until `Close` or the next `StartSession`, so one agent can run differently equipped sessions.
- A `SessionConfig.Skills` collection has its tools registered in the session's `ToolBox` and its catalog appended to the prompt; `Close` removes those tools again.
- Install a `Feedback` sink with `SetFeedback` to observe tool calls and session events; the default is silent. `ToolCalled(toolName string, args map[string]any)` fires just before each tool runs, with the arguments the model supplied — JSON-typed, so numbers are `float64`, and nil for a call with none. The map is the one the tool is about to run with, so a sink must read it, not modify it. `ToolReturned(toolName string, result string, err error, elapsed time.Duration)` fires just after the call, carrying what the tool returned — empty when `err` is non-nil — and how long it took. Tool calls run one at a time, so each `ToolReturned` pairs with the `ToolCalled` immediately before it. `TokensUsed(totalTokens int)` fires after each intermediate model response — one that carries tool calls — so usage can be tracked without waiting for the final answer, whose usage arrives in `Response.Metadata` instead.

## License

MIT — see [LICENSE](LICENSE).
