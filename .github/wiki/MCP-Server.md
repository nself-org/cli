# MCP Server

← [[Architecture]] · [[JSON-Output]] · [[Command-Registry]] · [[Compat-V15]]

---

This page describes how machine surfaces (MCP today, the HTTP API next) run nSelf commands. Both call one package, `internal/invoke`, so they cannot drift apart. `nself mcp` builds its tools, resources and search from the machine registry (see [Generated tools](#generated-tools)). The 26 hand-written tools stay primary in v1.4 mode and become deprecated names in v1.5 mode; see [[cmd-mcp]].

## Execution model

A machine request names a command from the **machine registry** and gives typed parameters. The invoker:

1. looks the command up in the registry (an unknown path is `E432`),
2. applies the exposure rules (a refusal is `E421`, with the reason in `cause`),
3. applies the confirmation gate (see [Gate](#gate)): the request runs, gets a confirmation back, or is refused (`E424`, `E425`),
4. builds an argv from the typed parameters (a bad request is `E420`),
5. runs `nself <path> --json ...` as a **child process of the same binary**, never in the server process,
6. returns the child's single JSON document and its exit code, or an error envelope (`E422` for no single document, `E423` for a timeout).

The child refuses to run anything but the command the invoker checked: the invoker sets `NSELF_INVOKE_PATH` to the classified path, and the child resolves its own argv with the same lookup it is about to execute and answers `E421` when the two differ (for example after a plugin upgrade gave a leaf new subcommands, or the binary was replaced, while the server kept its registry from start-up). The variable is removed before the command runs, so nothing the command spawns inherits it.

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

becomes `<path> --json --quiet=true --tags=a --tags=b -- -x`. Flags are sorted by name, a boolean is always `--name=true` or `--name=false` (never a bare `--name`: a mounted plugin node parses no flags, and cobra's command lookup would read a bare unknown flag as taking the next word, swallow the `--` and run the first positional as a subcommand), and a list is one `--name=value` per item. Positional values always come after `--`, so none of them can be read as a flag. A `stringSlice` flag is split on commas by the child, so an item that holds a comma, a quote or a newline is refused instead of being silently changed.

`argv` on a plugin command that has subcommands is `E421`: a mounted plugin node parses no flags, so the child would read the first word as a subcommand and run a node the exposure rules never checked. Its params schema does not offer `argv`. For a leaf, `argv` is the plugin's own input: the exposure decision covers the command at the requested path, not what the plugin binary does with its words. Accepted risk: free-form `argv` can carry the plugin's own undeclared flags (for example `--force`), which the side-effect classification cannot see, so a plugin leaf declared `read` that takes undeclared mutating flags can write. Plugin authors declare every flag with its side effect in the manifest to get the gate.

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
3. Neither. In v1.5 mode (`NSELF_V15=1`) the first loopback start generates 32 random bytes as 64 hex characters, creates `~/.nself` at `0700` if it is missing (an existing one is narrowed to `0700`, never widened; if that fails and group or others can write to it, the start is refused), writes the file at `0600` and prints its path and a `cat <path>` hint on standard error. The token value is never printed to any stream. Later starts read the file. In v1.4 mode (the default today) a start with no token serves an open loopback port and prints one warning.

Long-lived responses (SSE streams, a tool call that runs for minutes) are not cut by the read timeout: the request body is read under it, then the read deadline is cleared. A request without the right `Authorization: Bearer <token>` header gets `401 unauthorized` with `WWW-Authenticate: Bearer realm="nself-mcp"`. The comparison hashes both sides first, so it does not depend on the token length. With a token set, behaviour is the same in both modes. Both modes also read an existing token file, so a malformed or group-readable `~/.nself/mcp-token` stops a v1.4 start with `E426` too (the file is new in 1.5; nothing created it before). `stdio` needs no token. The invoker removes `NSELF_MCP_TOKEN` from the environment of the commands it runs.

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
- `--allowed-host` gates the bind. In v1.5 it also feeds the Host check: `sse` and `http` serve only a Host of `localhost`, `127.0.0.1`, `[::1]` or an `--allowed-host` name, each with the listener port (case-insensitive), and answer anything else `421` with `E428` before the token is checked. An invalid `--allowed-host` (a port, a URL, an unbracketed IPv6) exits `E430`. Behind a reverse proxy or TLS terminator in v1.5, forward Host as `<allowed-host>:<listener port>` (the same rule as `nself admin serve`); otherwise requests get `421` with `E428`. In v1.4 the Host header is not checked.
- An accepted non-loopback start prints one warning that the bearer token crosses the network over cleartext HTTP. The server has no TLS: put a TLS terminator in front of it.

The HTTP server sets `ReadHeaderTimeout` 10 s and `ReadTimeout` 30 s and no write timeout, so SSE streams stay open. The bearer check runs first, so an unauthenticated request is answered `401` before any body byte is read; an authenticated body is capped at 4 MiB (`413` above it). Headers, bodies and tokens are never logged.

## Gate

The gate decides on the effective side effect of the request (flag escalation applied). A command's registry `confirm` block names its consent flags (`--yes`, `--force`, ...) and, optionally, its plan form (`plan.flag`, `plan.id_flag`).

- Consent and plan-id flags belong to the invoker. A request that sets one is refused with `E425` before anything runs.
- `read` runs.
- `write` without a plan form runs, with no consent flags.
- A command with a plan form and no `confirm`: the invoker runs the plan form once (`<path> --json --<plan.flag> ...`) and reads `data.plan_id` and `data.requires_confirmation` from the child's change plan. When `requires_confirmation` is true, or the class is `remote` or `destructive`, it returns a confirmation of kind `plan` whose `confirm` is the plan id. Otherwise it runs the command once with `--<id_flag>=<plan_id>`. A plan exec that fails returns the child's error envelope unchanged; a plan without a valid `plan_id` or `requires_confirmation` is `E422`. The plan exec and the run are two children, each with the full invocation timeout.
- `remote` or `destructive` without a plan form and no `confirm`: a confirmation of kind `request`, whose `confirm` is a nonce.
- A server without a confirmation store refuses every remote and destructive command with `E421`.

The gate is not a prompt-injection control: an agent that was told to confirm will confirm. The controls are the token, the loopback default, request validation, the confirmation round trip and the audit log; the HTTP API adds Host and Origin checks (`nself mcp` checks the Host header in v1.5, not the Origin).

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

Accepted risks:

- A plan confirmation also passes the command's consent flags, as D3 states. For `build`, `--yes` also accepts the second render after plugin effects, so the plan id binds the first render only.
- The live nonce store is first in, first out. A client holding the token can issue 1024 confirmations and push out one that a person is about to approve; the round trip then starts again.

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
| `request` | `{args, argv, flags}` with secret-marked args and flags, flag values whose name looks secret (`config.IsSecretKey`: SECRET, PASS, KEY, TOKEN, CREDENTIAL, WEBHOOK, PRIVATE, DSN, SALT) and free-form argv items replaced by `"[REDACTED]"`, then the usual redaction on every string |
| `exit_code` | the child's exit status, or `null` for a confirmation or an invoker refusal |
| `error_code` | the invoker's `E42x`, the child's `error.code`, or `null` |
| `duration_ms` | wall time of the invocation |

Before any command that is not a read runs, the server checks that the audit file can be opened for append (a regular file, never a symlink); when it cannot (or the server has no audit log), the run is refused with `E421` and nothing changes. A writer waits at most 5 seconds for another process's lock on the file. A write that fails after the run (a full disk) prints one line with the operating-system error on the server's standard error (never the request) and does not stop the server. An existing log directory that group or others can read (other nself commands create `~/.nself/logs` at `0755`) is narrowed to `0700`.

## Security model

What the gate is: a consent step. A remote or destructive call first gets a confirmation back, then runs only when the same call is repeated with the nonce. That proves the caller saw the confirmation. It does not prove a person agreed, and it is not authorization: whoever holds the token (or runs the stdio process) can run every exposed command, and an agent told to confirm will confirm. Treat the token like a shell on the project.

What protects the server:

- **Token.** `sse` and `http` need the bearer token on every request (see [Machine token](#machine-token)). `stdio` has no network surface and no token. The token is never printed, never put in a tool result or the audit log, and removed from the environment of every command the invoker runs.
- **Loopback only by default.** A non-loopback `--host` is refused unless `--allowed-host` and a configured token are given.
- **Host and Origin.** `nself mcp` answers a Host it does not know with `421` (v1.5) and sends no CORS headers; the HTTP API also checks Origin. See [Transports and --host](#transports-and---host).
- **Request validation.** Every request value is exactly one argv element, after `--` for positional values. Unknown members, flags and tools are refused. A flag name, the consent flags and the root flags cannot be smuggled in as values.
- **Single-use nonces.** A nonce is bound to one request, lives 10 minutes, dies with the process and is spent by a confirmed call that reaches the nonce check, right or wrong. A request that fails validation first (for example a wrong argument count, `E420`) does not reach the check and does not spend it. A nonce of one tool never confirms another.
- **One operation per project.** Write, remote and destructive commands take the project operation lock in the child. When two confirmed calls reach one project, one runs and the other is refused with `E460` (HTTP `409`). The lock is taken by the child process, so it holds across servers and clients.
- **Audit log.** Every mutation and refusal is recorded (see [Audit log](#audit-log)); a run is refused when the log cannot be written.
- **Tool names and text.** A tool name is `nself_` plus the command path, and only `a-z`, `0-9` and `_`, at most 64 characters. A plugin command whose name is invalid, too long, or taken by an earlier tool is skipped with a warning on standard error and never stops the server; core commands claim names first. Summaries and flag descriptions from a plugin manifest are cleaned before they reach a tool description: controls, bidirectional and zero-width characters are removed, white space is collapsed and the text is cut at 500 characters. The side-effect sentence is always kept.

Known limits:

- Injection text in a plugin summary stays text. The generator bounds and cleans it; it cannot tell a hostile sentence from an honest one, and look-alike letters survive. Install plugins you trust.
- `nself://registry` returns the registry as the plugins declared it (JSON, not cleaned).
- The live nonce store holds 1024 entries, first in first out (see the round trip above).
- `nself mcp` does not check Origin. The Host check and the token are its defence against DNS rebinding.
- The lock covers commands that take it. A read never does.

The adversarial suite runs with `go test`: `TestMCPGenAdversarial` (`internal/mcpgen`: hostile names, descriptions, schemas, arguments), `TestMCPAdversarial` (`cmd/commands`: concurrent destructive calls, nonce reuse, token leaks, hostile plugins, smuggling), and the invoker, HTTP and machine-auth suites in their packages. The `Fuzz Nightly` workflow fuzzes `FuzzBuildArgv` and `FuzzDecodeRequest` (`internal/invoke`), `FuzzRoutePath` and `FuzzHostHeader` (`internal/httpapi`) and `FuzzToolName` (`internal/mcpgen`) for five minutes each on hosted runners. A crasher fails the job and its input is uploaded as an artifact; commit it as a seed with the fix.

## Generated tools

`internal/mcpgen` turns the machine registry into MCP tools. There is no hand-registered tool in v1.5 mode: every tool is a generated tool or a row of the legacy table.

| Part | Value |
|---|---|
| Name | `nself_` + the path words joined by `_`, `-` becoming `_` (`config secrets rotate` is `nself_config_secrets_rotate`). A name over 64 characters or a collision fails the start. |
| Which commands | every command the [exposure rules](#exposure-rules) serve over MCP, installed plugin commands included. A plugin subcommand that declares no side effect counts as destructive and has no confirm block, so it is not listed. |
| Input | the machine request schema (`args`, `flags`, `argv`, `confirm`), `additionalProperties: false`. Flags with the `output: stream` override are not offered; a call that sets one is `E421`. |
| Output | the command's committed data schema (`schemas/commands/`) as `outputSchema`; a command with none gets `{"type":"object"}`. A command that can ask for confirmation allows the confirmation shape too. Data that is not an object is wrapped as `{"value": ...}` in the schema and in the result. |
| Annotations | `readOnlyHint` for read, `destructiveHint` for destructive, `openWorldHint` for remote, taken at the worst class a flag can reach. |
| Result | `structuredContent` is the envelope `data`, the text content is the full envelope, `isError` is true when the envelope has `error`. A confirmation is a success: `isError` is false and `structuredContent.confirmation_required` is true (see [Confirmation round trip](#confirmation-round-trip)). |
| Errors | a bad request is an `E420` error envelope in the result, never a transport error. A document that is not an envelope is `E422`. |
| Plugin tools | a plugin command whose tool name is over 64 characters, is another generated tool's name or is a legacy alias name is skipped with a warning on standard error; the other tools are unaffected. A plugin name that only collides with the shortcut or an alias added after the generated tools still stops the start. The same fault in a core command stops the start. In v1.4 mode a start that cannot build the generated surface serves the 26 hand tools and prints why. |

The committed tool list for the hermetic tree (no plugin installed) is `internal/mcpgen/testdata/tools.golden.json`; `go test ./internal/mcpgen/ -run ToolListGolden -update` rewrites it.

## v1.4 and v1.5 modes

| | v1.4 mode (default) | v1.5 mode (`NSELF_V15=1`) |
|---|---|---|
| Legacy tools | the 26 hand tools, behaviour unchanged | none registered by hand |
| Generated tools | added under names no legacy tool uses | all of them |
| Same-name tools | the hand tool answers | one generated tool with a dual-shape input |
| Other legacy names | the hand tool answers | deprecated aliases |
| Resources | the four hand resources, plus the new ones | all served from the registry |
| Server info | `experimental: true` in the initialize capabilities | no flag |

No name is ever registered twice.

### Legacy tool names

`internal/mcpgen/legacy_aliases.yaml` has one row per legacy name.

**Merged (13).** `nself_status`, `nself_doctor`, `nself_logs`, `nself_config_get`, `nself_config_show`, `nself_config_set`, `nself_build`, `nself_start`, `nself_stop`, `nself_restart`, `nself_db_migrate_status`, `nself_backup_list`, `nself_deploy_status` are the generated name of their command. Their input schema is the machine request plus the old parameters as `Deprecated:` properties.

- The call is in the legacy shape when at least one old-only property is present. `{}` is the machine shape.
- Old properties map through the row (`nself_logs {service, lines}` runs `logs` with `args [service]` and `flags {tail: lines}`). A legacy call applies the old defaults (`lines` is 50) and the old required members (`key`, `value`); a missing one is `E420` naming it. The machine shape keeps the command's own defaults.
- A legacy call adds `meta.deprecations` (removal v1.6.0). Mixing machine members and old properties in one call is `E420`.
- `confirm` works in both shapes, so a legacy `nself_config_set` can still complete its confirmation.
- The hand tools' tolerance is kept in the legacy shape: an empty `value` clears a key, a JSON `null` counts as not given (so the default applies), and `lines` or `interval_seconds` of zero or less means the default (50 and 60). A non-integer `lines`, or one too large for 64 bits, is `E420`.
- The result changes from the old text to the command's `data`. This output-schema change is a v1.5 break, listed in [[Compat-V15]].

**Aliases (12).** `nself_urls` is `status urls`, `nself_service_list` is `config services list`, `nself_env_list` is `config env list`, `nself_plugin_list` is `config plugins list`, `nself_plugin_install` is `add`, `nself_get_schema` and `nself_get_permissions` are `db hasura schema` and `db hasura permissions`. The five `sentry_*` names map to the `sentry monitors`, `sentry incidents` and `sentry status` commands of the `sentry-cli` plugin; without that plugin a call is `E404` and names `nself add sentry-cli`. Every alias description starts with `Deprecated:`, and every call adds `meta.deprecations`.

**Shortcut (not a legacy name).** `nself_capabilities {query, limit}` is the short form of `nself_help_capabilities`. It is registered in both modes, is not deprecated and adds no deprecation note.

**Removed (1).** `nself_run_migration` ran raw SQL outside the migration ledger. A call is `E433` and names `nself db migrate create` and `nself db migrate up`.

### Legacy parameters

Two limits come from the exposure rules, which this change does not touch:

- `env` on `nself_config_get`, `nself_config_show` and `nself_config_set`. The `--env` flag belongs to the `config` parent, and the request schema offers only the flags a command declares itself. A call that sets `env` is `E420`. Select the environment with `nself config env use` before the call.
- A legacy name whose command is not exposed yet (no envelope: `start`, `stop`, `restart`, `config services list`, `config plugins list`, `add`) stays registered, its description says so, and a call returns the invoker's `E421`. It starts working when the command gets an envelope, with no change here.

## Resources

| URI | Content |
|---|---|
| `nself://registry` | the machine registry document with installed plugin commands, always in the v1.5 command spelling. In v1.5 mode it equals the data of `nself help --json`. |
| `nself://errors` | every registered error code with summary, cause, fix and exit class |
| `nself://schemas/{name}` | a committed schema by file name; `nself://schemas/commands/{name}` and `nself://schemas/invoke/{name}` reach the subfolders. The bytes are the committed ones. |
| `nself://plugins/catalog` | the plugin catalogue as the local cache holds it. The server never fetches: with no cache the read fails. |
| `nself://config`, `nself://services`, `nself://env`, `nself://urls` | in v1.5 mode the data of `config show`, `config services list`, `config env explain`, `status urls`, through the invoker. A URI whose command has no envelope yet keeps its hand handler. |

## Self-description

`nself help capabilities <query> [--limit 5]` searches the commands and the installable plugins by plain words, so an agent can find what to run. It is an ordinary registry command, so it is also the tool `nself_help_capabilities` and an HTTP route.

Each result is `{kind, path or slug, summary, run, score}`; `run` is the exact text (`nself config secrets rotate`, `nself add k8s`). A command scores 3 for each query word found in its path and 1 for each found in its summary. A plugin scores 2 for each query word found in its slug, summary, category or bundles. Words match when equal, when they share a stem (`diagnose`, `diagnostics`) or by a short synonym list (`database` and `db`, `kubernetes` and `k8s`). A parent command scores two below its best matching subcommand when that is higher. Plugin hits come from the cached catalogue only. `cmd/commands/testdata/capabilities/queries.json` records eight queries and the hit each must return in the top three.

## Prompts

The three prompts (`diagnose-failure`, `add-service`, `prepare-deploy`) are written by hand. A test checks that every `nself ...` command, tool name and resource they mention exists in the v1.5 registry and on the v1.5 server.


## Setup

`nself mcp setup` prints, or with `--write` writes, the stdio config an AI client needs to start the server for a project. The config holds no token: stdio needs none.

| `--client` | File (with `--write`) | Output |
|---|---|---|
| `claude-code` (default) | `.mcp.json` | `{"mcpServers":{"nself":{"command":"nself","args":["mcp"]}}}` |
| `cursor` | `.cursor/mcp.json` | the same JSON |
| `codex` | none (print-only) | a `[mcp_servers.nself]` TOML block for `~/.codex/config.toml` |
| `generic` | none (print-only) | the same JSON |

- Printing also puts a note on standard error: start the client in the project directory, because `nself mcp` finds the project from its working directory.
- `--write` creates the file or adds the `nself` entry to it. Every other key and server entry stays where it was, and an `nself` entry that already exists is left alone, so a second run changes nothing. A file that is not valid JSON is never overwritten. Files are written only under the project directory, and a symlink in the way is refused.
- `--client codex --write` and `--client generic --write` exit 1 with `E401`: copy the printed config yourself.
- `--json` prints one envelope; `data` is `{client, path, content, written}`.

### AGENTS.md

`nself mcp setup --agents-md` prints the project `AGENTS.md` rendered from the command registry: the canon verbs with their summaries, the two-command flow, the JSON and exit-code contract and the MCP setup line. The body holds nothing that varies per project or machine, so the same binary renders the same bytes everywhere (nself.org builds its own `AGENTS.md` from the pinned CLI this way).

`nself init` writes the same file without prompting and reports it with one line on standard error (nothing on standard output changes; `--quiet` stays silent). The first line is a marker with the SHA-256 of the body:

```
<!-- GENERATED BY nself (mcp setup) sha256:<hash of the body> — edit freely; nself will not overwrite an edited file -->
```

`init` and `mcp setup --agents-md --write` write the file only when it is absent or its body still matches the marker's hash. A file you edited, or one nself did not generate, is never touched.
