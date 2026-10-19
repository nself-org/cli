# nself plugin test

> Run a plugin's test suite (unit and smoke install/uninstall).

## Synopsis

```bash
nself plugin test <name> [flags]
```

## Description

`nself plugin test` runs a plugin's automated test suite. It supports two test phases:

- **unit** — runs the plugin's Go unit tests directly.
- **smoke** — performs a real install/uninstall cycle, verifying the plugin integrates correctly with the nSelf runtime.
- **both** (default) — runs unit then smoke.

The container image follows the `go` line of the plugin's `go.mod` (`golang:<major.minor>-alpine`, `golang:1.26-alpine` when there is none), with `GOTOOLCHAIN=auto` so a newer patch level is fetched.

Use `--host` to run tests in host-process mode instead of a container. Use `--no-cleanup` to preserve the post-test state for manual inspection when smoke tests fail.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--phase` | `both` | Test phase to run: `unit`, `smoke`, or `both` |
| `--host` | `false` | Run tests in host-process mode instead of container |
| `--no-cleanup` | `false` | Skip cleanup after smoke test (useful for debugging) |

## Examples

```bash
# Run full test suite (unit + smoke) for the ai plugin
nself plugin test ai

# Run unit tests only
nself plugin test ai --phase unit

# Run smoke test only, skip cleanup for debugging
nself plugin test ai --phase smoke --no-cleanup

# Run in host-process mode
nself plugin test ai --host
```

## See Also

- [[cmd-plugin-dev]] — Start a plugin in development mode
- [[cmd-plugin-debug]] — Attach a debugger to a running plugin
- [[cmd-plugin-link]] — Register a local plugin directory

← [[Commands]] | [[Home]] →
