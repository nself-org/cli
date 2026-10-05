# Safe On Live

← [[Home]]

---

How nself keeps a serving project safe from its own commands. This page covers the operation lock and the build change plan. Related pages: [[Compat-V15]], [[Exit-Codes]], [[error-codes]].

## Operation lock

One write, remote or destructive nself command runs at a time per project, across processes. A second one stops instead of racing the first over generated files, the `.env` cascade or the running stack.

### What takes it

The invocation decorator takes the lock around a command's body when all of these hold:

- A project root resolves from the current directory (the nearest folder with an `.env` marker file). Outside a project nothing is locked.
- The command's registry `side_effect`, after flag escalation, is `write`, `remote` or `destructive`. For example `nself build`, `nself config set`, `nself stop --volumes`, `nself plug --apply`.
- The command's output kind is `document`. `stream` and `interactive` commands are exempt: they can run for hours and no machine surface exposes them. `nself start --watch` is a stream.

Read commands (`status`, `doctor`, `config get`, `logs`, and every other `side_effect: read`) never take the lock and never create its file. A command refused by `--json` (E402) is refused before any lock is taken.

### How it works

The lock is an exclusive `flock` on `<project>/.nself/op.lock`. The kernel drops it when the holder exits or is killed, so a crash (including `kill -9`) never leaves it held. The file stays on disk; only the flock means "held".

While held, the file contains one JSON object (`contract:cli.oplock` v1):

```json
{"pid": 4242, "command": "nself build", "started_at": "2026-10-05T12:00:00Z", "token": "<32 hex>", "host": "devbox"}
```

The lock is released when the command returns, fails or panics, and when the process dies. `nself start --watch` releases it before its watch loop, so a concurrent `nself config set` proceeds.

### Contention

| Mode | A second command finds the lock held |
|---|---|
| v1.5 (`NSELF_V15=1`) | Fails at once with E460, exit 1, naming the holder: `wait for nself build (pid 4242) or stop it`. With `--json` the error envelope goes to stdout as usual. |
| v1.4 (default) | Waits up to 30 seconds (one notice on stderr), then prints a warning on stderr and runs without the lock. |

If the lock file cannot be created (read-only project folder) the command runs unlocked with a warning on stderr. The lock is a no-op on platforms without `flock` (Windows). Use WSL2 there. All notices go to stderr, never stdout.

### Nested nself commands

A command that runs `nself` itself (`nself bundle`, `nself admin`, `nself deploy`, `nself mcp` call `nself build`) must not deadlock on its own lock. The holder exports its token as `NSELF_OPLOCK_TOKEN`. A child `nself` whose token equals the token in the holder file, while the lock is held, runs without taking it. A token whose lock is no longer held grants nothing.

### What it does not do

It is a cooperative lock between nself processes of the same user:

- A process that does not use nself (a shell script, `docker compose` by hand, an editor) ignores it.
- The token is in the holder file, so any reader of the project folder can export it and pass for a child. It stops accidents, not an adversary.
- A child that outlives its parent runs unlocked: the parent's lock ended with the parent.
- A `clean` that deletes `.nself/` while the lock is held orphans that lock. The next command locks the new file.
- `nself build` takes this same lock (it no longer keeps a separate `.nself/build.lock` file), so a killed build leaves nothing stale. In v1.5 a build started while another process holds the lock fails and names the holder (E460 text); in v1.4 it waits up to 30 seconds, warns on stderr and builds unlocked, as the command guard does.

### Codes

| Code | Exit | Meaning |
|---|---|---|
| E460 | 1 | Another nself command holds the project operation lock. |
| E461-E464 | | Reserved. |

## Build change plan

`nself build --plan` answers "what would this change?" before anything changes. `nself build` with no flags then applies that same plan. Both run the one build pipeline: the plan is the pipeline run in memory, so nothing the apply does is missing from it, with the exceptions listed under "What a plan does not cover".

### What `--plan` guarantees

- It writes nothing under the project, `~/.nself`, the plugin directory, the fronting stack's nginx directory or `/etc/hosts`, and it starts, stops and creates no container. The command guard still creates the operation lock file `.nself/op.lock` (build is a `write` command), and the usual one-line invocation log in `~/.nself/logs/nself.log` is written unless `NSELF_CMD_LOG_ENABLED=false`.
- Container impact comes from `docker compose config --hash` over the planned files (the same files and env files `nself start` passes) compared with the `com.docker.compose.config-hash` label of the running containers. A service whose hash differs is `recreate`; one running without the label is `unknown` and treated as a recreate. A service that mounts a named volume is `stateful`. Every container item is `applied_by: next-start`: no build command recreates a container. If Docker cannot be asked, the plan says `containers.known: false` and prints a notice on stderr; it never fails for that reason.
- Secrets are never printed: env-kind artifacts are redacted, `--diff` shows key names only, and an effect that persists a secret names the key, not the value.

### The plan

The plan is the `contract:cli.change-plan` v1 document (schema `schemas/commands/build.v1.schema.json`): `plan_id`, `env` and `env_class`, `artifacts` (changed files only, with `diff_lines` and `generated`/`hand_edited`), `effects`, `containers`, `destructive` with its reasons, `requires_confirmation` and `empty`. `empty` is true when the build would change nothing, which is what `--plan` prints after an apply.

### plan_id

`plan_id` is the sha256 of the plan's canonical JSON followed by, for every artifact in path order, the sha256 of the bytes the build would leave there. The id binds the content of the change, not only its shape, while the JSON carries no file bytes. `nself build --yes --plan-id <id>` recomputes the plan and applies only if the id is unchanged; otherwise it fails with E450 and writes nothing. Swapping one env value for another of the same length changes the id. One exception: a run that generates secrets (a first build) renders values that differ per run into the env files, so those files are bound by name only for that run; once the secrets are persisted the next plan binds them fully.

### Prod-class confirmation

A prod-class env is `ENV` `prod` or `staging` (a running stack does not make an env prod-class). A non-empty plan there needs `--yes` or an interactive yes. A plan that overwrites or removes a hand-edited generated file needs `--force` (or an interactive yes) in any env. Both refusals are v1.5 behaviour:

| Mode | Prod-class change, no `--yes`, not interactive |
|---|---|
| v1.5 (`NSELF_V15=1`) | Refused with E403 (exit 4). With `--json` the error envelope is on stdout. Nothing is written, and the expired-plugin removal that `nself build` otherwise performs does not run. |
| v1.4 (default) | The plan summary and a one-line notice go to stderr, then the build proceeds as before. |

`--yes` does not authorise a hand-edited overwrite; that needs `--force`. A destructive plan (a hand-edited file overwritten, an orphan or expired plugin removed, a stateful container recreated by the command itself) is worded strongly in the prompt and listed under `destructive_reasons`.

### What a plan does not cover

- An expired plugin removed by the plugin lifecycle step is listed as a `plugin-remove` effect, but the plan renders the project with that plugin still installed, because the removal runs after the confirmation. The apply then renders without it.
- A declared plugin that `build` would auto-install is a `plugin-install` effect; the plan renders without it.
- The plan is computed under the operation lock held for the whole command, so the inputs cannot change between the plan and the write by another nself command. A process that ignores the lock can still change them.

### Codes

| Code | Exit | Meaning |
|---|---|---|
| E450 | 1 | `--plan-id` does not match the plan this project produces now. Re-run `nself build --plan` and pass the new id. |
| E403 | 4 | A prod-class or hand-edited change was not confirmed (v1.5). |
