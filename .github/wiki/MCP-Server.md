# MCP Server

← [[Architecture]] · [[JSON-Output]] · [[Command-Registry]] · [[Compat-V15]]

---

This page describes how machine surfaces (MCP today, the HTTP API next) run nSelf commands. Both call one package, `internal/invoke`, so they cannot drift apart. The legacy hand-written MCP tools in `nself mcp` keep their behaviour until the generated tools replace them; see [[cmd-mcp]].

## Execution model

A machine request names a command from the **machine registry** and gives typed parameters. The invoker:

1. looks the command up in the registry (an unknown path is `E432`),
2. applies the exposure rules (a refusal is `E421`, with the reason in `cause`),
3. applies the confirmation gate (see [Gate](#gate)): the request runs, gets a confirmation back, or is refused (`E424`, `E425`),
4. builds an argv from the typed parameters (a bad request is `E420`),
5. runs `nself <path> --json ...` as a **child process of the same binary**, never in the server process,
6. returns the child's single JSON document and its exit code, or an error envelope (`E422` for no single document, `E423` for a timeout).

Running as a child keeps global state, `os.Exit` and the project operation lock identical to a typed command. There is no shell anywhere: every request value is exactly one argv element.

The **machine registry** is the v1.5 registry of the running binary with installed plugin commands mounted. It is the same in both compat modes ([[Compat-V15]]), so tool names and routes are the v1.5 shape in 1.4.x too. The plugin set is read when the server starts; install a plugin, then restart the server.

## Exposure rules

A command is served only when all of these hold:

- it is runnable, not hidden, not deprecated, and its canon status is `core`, `subcommand` or `plugin` (not `pending`, `deprecated-shim` or `builtin`),
- its registry `surface` is not `cli-only` (for example `config export`),
- its JSON support is `envelope`,
- its effective output is `document`. Stream commands are served only as HTTP NDJSON, and only when the effective side effect is `read`,
- its effective side effect is `read` or `write`, or it is `remote` or `destructive` and its registry row has a `confirm` block.

The effective side effect and output apply the flags a request sets: a flag that escalates a command to `destructive`, or that switches a document command to `stream`, changes the verdict for that request.

A remote or destructive command without a `confirm` block is refused with `E421` ("no gate"). One with a block goes through the confirmation round trip below.

Flags that are never exposed: `--json`, `--help`, the root switches, hidden, deprecated and `cli_only` flags (every `--reveal`), the consent and plan-id flags of a `confirm` block, and, in document mode, a flag that carries the `output: stream` override. Secret values never leave the process: a flag or argument marked `secret` in the registry is replaced by `[REDACTED]` in the request id and in any `E422` stderr tail.

## The machine request

A request is a JSON object. Every member is optional. Member names are matched exactly (`args`, not `ARGS`), a repeated member name (at the top level or inside `flags`) is `E420`, and so is an unknown member.

| Member | Type | Meaning |
|---|---|---|
| `args` | array of strings | positional values, in declaration order |
| `flags` | object | flag name to boolean, integer, number, string or array of strings |
| `argv` | array of strings | arguments for a plugin command that declares no args and no flags and has no subcommands (a leaf); items naming a root flag such as `--no-monorepo` are `E420` because the plugin proxy would drop them |
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

`argv` on a plugin command that has subcommands is `E421`: a mounted plugin node parses no flags, so the child would read the first word as a subcommand and run a node the exposure rules never checked. Its params schema does not offer `argv`. For a leaf, `argv` is the plugin's own input: the exposure decision covers the command at the requested path, not what the plugin binary does with its words.

Refused with `E420`, naming the flag but never echoing its value: an unknown or unexposed flag, a value of the wrong type (`1.5` for an integer), too few or too many args, a NUL byte anywhere, an element over 32 KiB or an argv over 128 KiB.

## Request id

The request id names a request in logs. It is the lowercase hex SHA-256 of the canonical JSON `{"args":[...],"argv":[...],"command":"<path>","flags":{...}}`: keys sorted, no whitespace, `confirm` left out, missing members empty, and every secret argument and flag value replaced by `"[REDACTED]"` first. Redaction fails closed: a positional past the declared args and a flag the command does not declare are replaced too. Free-form plugin `argv` items cannot be marked secret, so every one is replaced by `"[REDACTED]"` before hashing (the id keeps their count) and scrubbed from `E422` stderr tails. Two requests that differ only in a secret value share an id, and no id is derived from a secret. The id is not a confirmation: anyone can compute it.

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

A document request times out after 300 seconds by default (the invoker takes the timeout as an option; the servers expose it as `--timeout`). The child leads its own process group. The group gets SIGTERM, and SIGKILL five seconds later or as soon as the child is gone, so grandchildren (docker compose, ssh, helpers) do not outlive a timed-out or cancelled request; the result is `E423`. An NDJSON request has no timeout and ends when the client disconnects. A long-polling command such as `nself ci runs wait` reads `NSELF_INVOKE_DEADLINE_MS` and returns before the kill.

The child's standard output is capped at 32 MiB and must be exactly one JSON object. Anything else is `E422` (exit class infra); its `cause` is the last 2 KiB of the child's standard error, with the request's secret values replaced and the usual redaction applied. A child envelope that carries `meta.deprecations` or `meta.warnings` passes through byte for byte.

## Machine token

The `sse` and `http` transports of `nself mcp` check a bearer token on every request. The token comes from, in order:

1. `NSELF_MCP_TOKEN` (source `env`).
2. The file `~/.nself/mcp-token` (source `file`). It must be a regular file (a symlink is refused), mode `0600` on macOS and Linux, and not empty. A file that group or others can read is refused with `E426`; the message names the path and `chmod 600`.
3. Neither. In v1.5 mode (`NSELF_V15=1`) the first loopback start generates 32 random bytes as 64 hex characters, creates `~/.nself` at `0700` if it is missing (an existing directory is never chmod-ed; one that group or others can write to is refused), writes the file at `0600` and prints its path and a `cat <path>` hint on standard error. The token value is never printed to any stream. Later starts read the file. In v1.4 mode (the default today) a start with no token serves an open loopback port and prints one warning.

A request without the right `Authorization: Bearer <token>` header gets `401 unauthorized` with `WWW-Authenticate: Bearer realm="nself-mcp"`. The comparison hashes both sides first, so it does not depend on the token length. With a token set, behaviour is the same in both modes. `stdio` needs no token. The invoker removes `NSELF_MCP_TOKEN` from the environment of the commands it runs.

## Transports and --host

| Flag | Default | Meaning |
|---|---|---|
| `--transport` | `stdio` | `stdio`, `sse` or `http`. |
| `--port` | `3825` | Port for `sse` and `http`. |
| `--host` | `127.0.0.1` | Bind address for `sse` and `http`. |
| `--allowed-host` | none | A host name the server is reachable under. Repeatable. |

`stdio` ignores `--host` and `--allowed-host`. On `sse` and `http` the checks below run before the listener opens, in both compat modes, and a refused start writes no token file:

- Loopback hosts (`localhost`, any `127.0.0.0/8` address, `::1`) are always allowed.
- Any other host, including the empty host, `0.0.0.0` and `::`, exits non-zero with `E426` unless at least one `--allowed-host <name>` is given and a token is configured on that run through `NSELF_MCP_TOKEN` or an existing `~/.nself/mcp-token`. A token that this run would generate does not count, and a second start after a generated run is refused the same way.
- An accepted non-loopback start prints one warning that the bearer token crosses the network over cleartext HTTP. The server has no TLS: put a TLS terminator in front of it.

The HTTP server sets `ReadHeaderTimeout` 10 s and `ReadTimeout` 30 s and no write timeout, so SSE streams stay open. Headers, bodies and tokens are never logged.

## Gate

The gate decides on the effective side effect of the request (flag escalation applied). A command's registry `confirm` block names its consent flags (`--yes`, `--force`, ...) and, optionally, its plan form (`plan.flag`, `plan.id_flag`).

- Consent and plan-id flags belong to the invoker. A request that sets one is refused with `E425` before anything runs.
- `read` runs.
- `write` without a plan form runs, with no consent flags.
- A command with a plan form and no `confirm`: the invoker runs the plan form once (`<path> --json --<plan.flag> ...`) and reads `data.plan_id` and `data.requires_confirmation` from the child's change plan. When `requires_confirmation` is true, or the class is `remote` or `destructive`, it returns a confirmation of kind `plan` whose `confirm` is the plan id. Otherwise it runs the command once with `--<id_flag>=<plan_id>`. A plan exec that fails returns the child's error envelope unchanged; a plan without a valid `plan_id` or `requires_confirmation` is `E422`.
- `remote` or `destructive` without a plan form and no `confirm`: a confirmation of kind `request`, whose `confirm` is a nonce.
- A server without a confirmation store refuses every remote and destructive command with `E421`.

The gate is not a prompt-injection control: an agent that was told to confirm will confirm. The controls are the token, the loopback default, Host and Origin checks, request validation, the confirmation round trip and the audit log.

## Confirmation round trip

A confirmation is a success envelope (HTTP 428 on the HTTP API, `isError: false` over MCP) whose `data` is:

```json
{"confirmation_required": true, "kind": "request", "confirm": "<64 hex>", "side_effect": "destructive", "reason": "...", "plan": null}
```

For kind `plan`, `plan` is the child's change plan. Nothing has changed yet: the plan exec reads only.

To go ahead, send the identical request again with `"confirm": "<value>"`.

- Kind `plan`: the command runs with its consent flags and `--<id_flag>=<confirm>`. It recomputes its plan and refuses with `E450` when the state moved since the plan; that envelope comes back unchanged. Start again without `confirm` to get the new plan.
- Kind `request`: the nonce is 32 random bytes from `crypto/rand`, held in the server's memory with a digest of the full, unredacted request (so a different secret value is a different request), valid for 10 minutes and good for one run. The confirmed call must present a live nonce for the identical request; the nonce is then spent and the command runs with its consent flags. A reused, expired or unknown nonce, a value the client computed (the request id included), or a different request is `E424`, and the nonce is spent either way. Nonces die with the server process: after a restart, start the round trip again. The server keeps at most 1024 live nonces.

Mutations serialise through the project operation lock the child takes; contention in v1.5 mode is the child's `E460`.

## Audit log

Every non-read invocation and every refusal is appended to `~/.nself/logs/machine-audit.jsonl` (contract `cli.machine-audit` v1). Reads that run are not logged. The directory is created `0700` and the file `0600`; writers append under an exclusive file lock, and the file rotates at 10 MiB to `.1`, `.2`, `.3` (older ones are dropped).

One compact JSON object per line, keys in this order:

| Key | Value |
|---|---|
| `time` | RFC 3339 UTC with milliseconds |
| `transport` | `mcp-stdio`, `mcp-sse`, `mcp-http` or `http` |
| `project` | absolute project directory |
| `command` | registry path |
| `side_effect` | effective side effect |
| `decision` | `run`, `confirmation_required` or `refused` |
| `confirm_kind` | `plan`, `request` or `null` |
| `request_id` | the request id (nonces are never logged) |
| `request` | `{args, argv, flags}` with secret-marked args and flags, flag values whose name looks secret (`config.IsSecretKey`: SECRET, PASSWORD, KEY, TOKEN) and free-form argv items replaced by `"[REDACTED]"`, then the usual redaction on every string |
| `exit_code` | the child's exit status, or `null` for a confirmation or an invoker refusal |
| `error_code` | the invoker's `E42x`, the child's `error.code`, or `null` |
| `duration_ms` | wall time of the invocation |

A failed audit write prints one line with the operating-system error on the server's standard error (never the request) and does not stop the server.
