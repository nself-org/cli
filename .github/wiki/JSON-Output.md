# JSON Output

← [[Config-Env-Vars]] · [[Compat-V15]] · [[error-codes]]

---

`--json` is one persistent flag on the root command. It has no shorthand. A command either honours it or refuses it; it never prints text and calls that JSON.

```bash
nself <command> --json          # JSON mode
nself <command> --json=false    # explicitly off, same as leaving it out
```

`nself <command> --help` lists `--json` under Global Flags.

## The rule

In JSON mode:

- **stdout** carries exactly one JSON document and nothing else.
- **stderr** carries everything meant for a person: progress, warnings, deprecation notices, the monorepo notice (`Detected monorepo layout…`) and the human rendering of an error.

A script can always parse stdout once and get either data or an error.

## The envelope (v1)

Success:

```json
{
  "schema_version": "1",
  "command": "config get",
  "data": { "key": "PROJECT_NAME", "value": "demo" }
}
```

Error:

```json
{
  "schema_version": "1",
  "command": "status",
  "error": {
    "code": "E401",
    "message": "unknown flag: --bogus",
    "class": "user",
    "exit_code": 1
  }
}
```

Exactly one of `data` and `error` is present. An optional last member `meta` carries `deprecations` and `warnings` and is omitted when empty. `command` is the canonical command path without `nself `. Key order is fixed, indentation is two spaces, output ends with a newline.

Each command's `data` has its own schema (the `data_schema` field of the command registry). A breaking change to a `data` shape needs a new schema version.

## Commands that cannot produce JSON: E402

Every command has a JSON support value in the command registry (`internal/canon/canon.yaml`):

| Value | Meaning |
|---|---|
| `envelope` | prints the v1 envelope |
| `legacy` | prints its own bare JSON, unchanged; non-silent errors are still enveloped |
| `none` | cannot produce JSON |

`--json` on a `none` command (or together with a flag whose JSON override is `none`) fails before the command does any work:

```text
$ nself restart --json
Error: [E402] nself restart does not support --json
```

Exit status is `1`, stdout is empty, and nothing is restarted. The same applies to the bare `nself --json`. Commands that print secrets, such as `nself secrets decrypt-on-deploy`, are `none`: JSON mode never emits secret values.

## Compatibility (ADR 0021)

Behaviour that would break an existing script is dormant in 1.4.x and becomes the default at v1.5.0. Opt in early with `NSELF_V15=1` ([[Compat-V15]]).

| Case | v1.4 mode (default in 1.4.x) | v1.5 mode (`NSELF_V15=1`) |
|---|---|---|
| `--json` on a `none` command that had no `--json` before (for example `restart`) | `E402`, exit 1 (it was an unknown-flag error) | same |
| `--json` on a `none` command whose own `json` flag was accepted and ignored (for example `config validate`) | unchanged: accepted and ignored | `E402`, exit 1 |
| Unknown flag or bad arguments | cobra's message, unchanged | wrapped as `E401` |

## Legacy JSON and NSELF_JSON_LEGACY

Some commands printed their own JSON before the envelope existed (`status`, `doctor`, `config show --format json`). In v1.4 mode they keep printing it. In v1.5 mode they print the envelope; set `NSELF_JSON_LEGACY=1` (or `true`, any case) to restore the old bare JSON of just those commands for one minor release. A one-line deprecation warning goes to stderr once per process. `NSELF_JSON_LEGACY` is removed in v1.6.0. Error envelopes and every other envelope are unaffected.

## Plugins

A plugin-proxied command receives `--json` unchanged, so `nself sentry status --json` passes `--json` to the plugin. The plugin owns its output. The CLI's other global flags (`--no-monorepo`, `--no-deprecation-warnings`) are still stripped.

## See also

- [[error-codes]] for `E401` (invalid usage) and `E402` (JSON output not supported).
- The command registry (`.github/command-registry.json`, published by `nself help --json`) lists the JSON support of every command. Its own page lands with P7-REG-07.
