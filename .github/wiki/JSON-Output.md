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
- **Secrets.** Commands that print secret values, such as `nself secrets decrypt-on-deploy`, never put the values in JSON. In v1.5 `nself config secrets decrypt-on-deploy --json` lists the key names with every value `[REDACTED]`; the plaintext needs a terminal stdout or `--reveal` (text only).

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

## Schemas

Every JSON contract is published as a JSON Schema (draft 2020-12) under `schemas/` in the repository. The files are generated from the Go types by `tools/schemagen`; never edit one by hand. `make schemas` regenerates them and `make schemas-check` (also `go run ./tools/schemagen -check`) fails when a committed file is stale.

| File | Describes |
|---|---|
| `schemas/envelope.v1.schema.json` | The envelope: exactly one of `data` or `error`, `schema_version` `"1"`, optional `meta` (`deprecations[]` with `old`, `new`, `removal_at`; `warnings[]`) |
| `schemas/error.v1.schema.json` | The error object: `code` (`E` and three digits), `message`, optional `cause`, `remediation`, `docs_url`, `exit_code`, `class` |
| `schemas/command-registry.v1.schema.json` | The command registry document (`nself help --json` data and `.github/command-registry.json`) |
| `schemas/commands/<command>.v1.schema.json` | The `data` of one envelope command; the path is the command with spaces as `-` (`help`, `status`, `backup create` as `backup-create`) |
| `schemas/invoke/confirmation.v1.schema.json` | The `data` of a machine confirmation (contract `cli.machine-request` v1) |
| `schemas/deploy-targets.v1.schema.json` | The `data` of `nself deploy targets --json` |
| `schemas/bundle-ledger.v1.schema.json` | The bundle install-state ledger (contract `cli.bundle-ledger` v1) |
| `schemas/catalog.v1.schema.json` | The plugin catalog (contract `plugins.catalog` v1): one row per slug and tier, bundles and counts |
| `schemas/releases.v1.schema.json` | `releases.json`: version, sha256, signature and tarball, written only by the release pipeline |
| `schemas/control-plane.v2.schema.json` | The control-plane inventory in `.nself/control-plane.yaml` (secrets never inlined) |
| `schemas/lockfile.v1.schema.json` | The project lock `nself.lock.yaml` (contract `cli.lockfile` v1), as its parsed document |
| `schemas/nself-yaml.v1.schema.json` | `nself.yaml`, the project manifest `nself build` reads |
| `schemas/plugin-manifest.v2.schema.json` | The plugin manifest v2 (`plugin.json` with `manifest_version` 2) |
| `schemas/portable-export.v1.schema.json` | The portable bundle manifest (contract `cli.portable-export` v1) |
| `schemas/proxy-routes.v1.schema.json` | The provider-neutral model of generated proxy routes |
| `schemas/index.json` | Every generated schema with its `$id` |

`make contract-check` fails when a schema in `schemas/index.json` has no row here or a row names a file that does not exist (`scripts/contract-check-wiki.sh`).

Each schema has the `$id` `urn:nself:cli:schema:<name>:v1`. The envelope refers to the error schema by that `$id`, so a validator needs `error.v1` loaded alongside `envelope.v1` (the repository tests resolve it from `schemas/index.json`). Any 2020-12 validator works. To check a command's `data` with `check-jsonschema`:

```bash
nself help --json | jq .data > data.json
check-jsonschema --schemafile schemas/commands/help.v1.schema.json data.json
```

A breaking change to a schema needs a new `v2` file, not an edit of `v1`. Golden fixtures for every envelope command live in `cmd/commands/testdata/json/`; the contract test validates each against its schema and round-trips it through the Go type.

## Coming in v1.5

These are part of the design but not shipped yet; the pages and flags below do not exist in 1.4.x.

- **The v1 envelope** on stdout: `{"schema_version":"1","command":"<path>","data":…}` for success, `"error":…` instead of `data` on failure, with stdout carrying exactly one JSON document. It lands with P7-REG-06 (error envelope, exit classes), P7-REG-07 (`nself help --json`) and P7-REG-09 (`status`, `doctor`, `config` pilots). Until then a refused or failed `--json` command prints text on stderr only.
- **`NSELF_JSON_LEGACY=1`** (or `true`): once the envelope is the default in v1.5 mode, it restores the old bare JSON of `status`, `doctor` and `config show --format json` for one minor release, with a one-line warning on stderr. Removed in v1.6.0.
- **The command registry** (`.github/command-registry.json`, published by `nself help --json`) listing the JSON support of every command; its own page lands with P7-REG-07. Today `nself help --json` is refused like any other `none` command.

## See also

- [[error-codes]] for `E401` (invalid usage) and `E402` (JSON output not supported).
