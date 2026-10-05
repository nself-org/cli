# Safe On Live

← [[Home]]

---

How nself keeps a serving project safe from its own commands. This page covers the operation lock. Related pages: [[Compat-V15]], [[Exit-Codes]], [[error-codes]].

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
- The build step keeps its own `.nself/build.lock` for now; P7-LIVE-03 moves it onto flock.

### Codes

| Code | Exit | Meaning |
|---|---|---|
| E460 | 1 | Another nself command holds the project operation lock. |
| E461-E464 | | Reserved. |
