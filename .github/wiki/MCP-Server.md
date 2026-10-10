# MCP Server

← [[Architecture]] · [[JSON-Output]] · [[Command-Registry]] · [[Compat-V15]]

---

This page describes how machine surfaces (MCP today, the HTTP API next) run nSelf commands. Both call one package, `internal/invoke`, so they cannot drift apart. The legacy hand-written MCP tools in `nself mcp` keep their behaviour until the generated tools replace them; see [[cmd-mcp]].

## Execution model

A machine request names a command from the **machine registry** and gives typed parameters. The invoker:

1. looks the command up in the registry (an unknown path is `E432`),
2. applies the exposure rules (a refusal is `E421`, with the reason in `cause`),
3. builds an argv from the typed parameters (a bad request is `E420`),
4. runs `nself <path> --json ...` as a **child process of the same binary**, never in the server process,
5. returns the child's single JSON document and its exit code, or an error envelope (`E422` for no single document, `E423` for a timeout).

Running as a child keeps global state, `os.Exit` and the project operation lock identical to a typed command. There is no shell anywhere: every request value is exactly one argv element.

The **machine registry** is the v1.5 registry of the running binary with installed plugin commands mounted. It is the same in both compat modes ([[Compat-V15]]), so tool names and routes are the v1.5 shape in 1.4.x too. The plugin set is read when the server starts; install a plugin, then restart the server.

## Exposure rules

A command is served only when all of these hold:

- it is runnable, not hidden, not deprecated, and its canon status is `core`, `subcommand` or `plugin` (not `pending`, `deprecated-shim` or `builtin`),
- its registry `surface` is not `cli-only` (for example `config export`),
- its JSON support is `envelope`,
- its effective output is `document`. Stream commands are served only as HTTP NDJSON, and only when the effective side effect is `read`,
- its effective side effect is `read` or `write`.

The effective side effect and output apply the flags a request sets: a flag that escalates a command to `destructive`, or that switches a document command to `stream`, changes the verdict for that request.

Remote and destructive commands are refused with `E421` ("no gate") until the confirmation gate ships. The gate will accept a plan id or a server-issued single-use nonce; it is not part of this page yet.

Flags that are never exposed: `--json`, `--help`, the root switches, hidden, deprecated and `cli_only` flags (every `--reveal`), the consent and plan-id flags of a `confirm` block, and, in document mode, a flag that carries the `output: stream` override. Secret values never leave the process: a flag or argument marked `secret` in the registry is replaced by `[REDACTED]` in the request id and in any `E422` stderr tail.

## The machine request

A request is a JSON object. Every member is optional and an unknown member is `E420`.

| Member | Type | Meaning |
|---|---|---|
| `args` | array of strings | positional values, in declaration order |
| `flags` | object | flag name to boolean, integer, number, string or array of strings |
| `argv` | array of strings | verbatim arguments; only for a plugin command that declares no args and no flags |
| `confirm` | string, 64 hex characters | a plan id or a server-issued nonce (commands with a `confirm` block only) |

The params schema of each command (JSON Schema 2020-12, `additionalProperties: false`) is derived from the registry: `prefixItems` per declared argument, one typed property per exposed flag, defaults from the registry.

Example, a read:

```json
{"args": ["BASE_DOMAIN"]}
```

runs `nself config get --json -- BASE_DOMAIN` and returns the same bytes as `nself config get BASE_DOMAIN --json`.

Example, flags and a value that looks like a flag:

```json
{"args": ["-x"], "flags": {"tags": ["a", "b"], "quiet": true}}
```

becomes `<path> --json --quiet --tags=a --tags=b -- -x`. Flags are sorted by name, a boolean `true` is `--name`, `false` is `--name=false`, and a list is one `--name=value` per item. Positional values always come after `--`, so none of them can be read as a flag. A `stringSlice` flag is split on commas by the child, so an item that holds a comma, a quote or a newline is refused instead of being silently changed.

Refused with `E420`, naming the flag but never echoing its value: an unknown or unexposed flag, a value of the wrong type (`1.5` for an integer), too few or too many args, a NUL byte anywhere, an element over 32 KiB or an argv over 128 KiB.

## Request id

The request id names a request in logs. It is the lowercase hex SHA-256 of the canonical JSON `{"args":[...],"argv":[...],"command":"<path>","flags":{...}}`: keys sorted, no whitespace, `confirm` left out, missing members empty, and every secret argument and flag value replaced by `"[REDACTED]"` first. Two requests that differ only in a secret value share an id, and no id is derived from a secret. The id is not a confirmation: anyone can compute it.

## Child environment

The child gets the server's environment with these changes:

| Variable | Value |
|---|---|
| `NSELF_MCP_TOKEN` | removed |
| `NSELF_V15` | `1` |
| `NSELF_NONINTERACTIVE` | `1` |
| `CI` | `1` |
| `NO_COLOR` | `1` |
| `NSELF_INVOKED_BY` | `mcp` or `http` |
| `NSELF_INVOKE_DEADLINE_MS` | milliseconds left until the timeout, set at exec time; absent for NDJSON requests |

Any inherited value of these names is dropped first. The working directory is the server's project directory and the child has no standard input. Arguments are visible to other local users in the process list while the child runs, as for any command typed in a terminal.

## Timeout and output limits

A document request times out after 300 seconds by default (the invoker takes the timeout as an option; the servers expose it as `--timeout`). The child gets SIGTERM, and SIGKILL five seconds later; the result is `E423`. An NDJSON request has no timeout and ends when the client disconnects. A long-polling command such as `nself ci runs wait` reads `NSELF_INVOKE_DEADLINE_MS` and returns before the kill.

The child's standard output is capped at 32 MiB and must be exactly one JSON object. Anything else is `E422` (exit class infra); its `cause` is the last 2 KiB of the child's standard error, with the request's secret values replaced and the usual redaction applied. A child envelope that carries `meta.deprecations` or `meta.warnings` passes through byte for byte.
