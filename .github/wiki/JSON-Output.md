# JSON Output

← [[Config-Env-Vars]] · [[Compat-V15]] · [[error-codes]]

---

`--json` is one persistent flag on the root command. It has no shorthand. A command either honours it or refuses it; it never prints text and calls that JSON.

```bash
nself <command> --json          # JSON mode
nself <command> --json=false    # explicitly off, same as leaving it out
```

`nself <command> --help` lists `--json` under Global Flags.

## What ships today

- **Refusal.** `--json` on a command that cannot produce JSON fails before the command does any work: exit status `1`, nothing on stdout, and a text error on stderr (see E402 below).
- **Commands with their own JSON** (`legacy`, such as `status` and `version`) keep printing it, unchanged.
- **stdout and stderr.** The monorepo notice (`Detected monorepo layout…`) goes to stderr when `--json` is on, so it never mixes into stdout. Without `--json` it stays on stdout.
- **Plugins.** A plugin-proxied command receives `--json` unchanged (`nself sentry status --json` passes `--json` to the plugin) and owns its output. The CLI's other global flags (`--no-monorepo`, `--no-deprecation-warnings`) are still stripped.
- **Secrets.** Commands that print secret values, such as `nself secrets decrypt-on-deploy`, have no JSON support, so JSON mode never emits them.

## Commands that cannot produce JSON: E402

Every command has a JSON support value in the command registry (`internal/canon/canon.yaml`): `envelope` (the v1 envelope), `legacy` (its own bare JSON) or `none`. `--json` on a `none` command, or together with a flag whose JSON override is `none`, is refused:

```text
$ nself restart --json
Error: [E402] nself restart does not support --json
```

The refusal happens before the command's own start-up work, so nothing is restarted, fetched or cached. The bare `nself --json` is refused the same way.

## Compatibility (ADR 0021)

Behaviour that would break an existing script is dormant in 1.4.x and becomes the default at v1.5.0. Opt in early with `NSELF_V15=1` ([[Compat-V15]]).

| Case | v1.4 mode (default in 1.4.x) | v1.5 mode (`NSELF_V15=1`) |
|---|---|---|
| `--json` on a `none` command that had no `--json` before (for example `restart`) | `E402`, exit 1 (it was an unknown-flag error) | same |
| `--json` on a `none` command whose own `json` flag was accepted and ignored (for example `config validate`) | unchanged: accepted and ignored | `E402`, exit 1 |
| Unknown flag or bad arguments | cobra's message, unchanged | coded `E401`; values you typed (for example after `-x…` or a secret-named flag) are removed from the message |

## Coming in v1.5

These are part of the design but not shipped yet; the pages and flags below do not exist in 1.4.x.

- **The v1 envelope** on stdout: `{"schema_version":"1","command":"<path>","data":…}` for success, `"error":…` instead of `data` on failure, with stdout carrying exactly one JSON document. It lands with P7-REG-06 (error envelope, exit classes), P7-REG-07 (`nself help --json`) and P7-REG-09 (`status`, `doctor`, `config` pilots). Until then a refused or failed `--json` command prints text on stderr only.
- **`NSELF_JSON_LEGACY=1`** (or `true`): once the envelope is the default in v1.5 mode, it restores the old bare JSON of `status`, `doctor` and `config show --format json` for one minor release, with a one-line warning on stderr. Removed in v1.6.0.
- **The command registry** (`.github/command-registry.json`, published by `nself help --json`) listing the JSON support of every command; its own page lands with P7-REG-07. Today `nself help --json` is refused like any other `none` command.

## See also

- [[error-codes]] for `E401` (invalid usage) and `E402` (JSON output not supported).
