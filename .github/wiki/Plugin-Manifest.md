# Plugin Manifest v2

← [[Plugin-Architecture]]

---

`plugin.json` with `"manifest_version": 2` is the one authored plugin schema. It is generated from the Go types in `internal/plugin/manifestv2` and published as `schemas/plugin-manifest.v2.schema.json` (JSON Schema draft 2020-12). A file without `manifest_version` (or with `1`) is v1: the CLI still reads it through one normalizer and prints a one-line deprecation notice per process in v1.5 mode ([[Compat-V15]]).

Released 1.4.x CLIs decode `plugin.json` permissively, so v2 never reuses a v1 key with a different JSON type or a narrower enum. The CLI block is `commands` (v1 `cli` is a string), the runtime block is `service` (v1 `runtime` is a string), and the lifecycle enum is `maturity` (v1 `status` has a fixed enum).

## Example

```json
{
  "manifest_version": 2,
  "name": "tenant",
  "version": "1.0.0",
  "description": "Multi-tenant operations",
  "category": "infrastructure",
  "license": "free",
  "maturity": "implemented",
  "service": {"kind": "cli"},
  "commands": {"command": "tenant", "binary": "nself-tenant", "summary": "Manage tenants"}
}
```

Add the generated compatibility keys with `manifestv2migrate -compat -write` (below).

## Keys

Three groups share one flat object:

- **Canonical v2 keys**: `manifest_version`, `license`, `maturity`, `installable`, `requires`, `service`, `commands`, `schema`, `migrations`, `seed`, `env`, `routes`, `docs_url`.
- **Shared v1 keys**: authored with the v1 name and JSON type and read natively (for example `port`, `dependencies`, `optionalDependencies`, `visibility`, `tables`, `permissions`, `envVars`, `deprecation`, `requiredEntitlements`). The list is the set of keys a v1.4.12 reader decodes, committed as `tools/manifestv2migrate/testdata/v1412-readers.txt`. Polymorphic v1 shapes (grouped `dependencies`, object-form `webhooks`) are written in their one canonical shape.
- **Compatibility keys**: generated, never authored (below).

Bundle membership is not a manifest field: `bundles.json` is the membership source. `bundles`, `tier_pair`, `author_public_key`, `signature` and `checksum` are rejected in a v2 file (E111).

<!-- GENERATED:fields BEGIN (tools/manifestv2migrate -wiki; DO NOT HAND EDIT) -->

| Key | Type | Required | Rule |
|---|---|---|---|
| `apiEndpoints` | array or null | no |  |
| `arch_support` | array or null | no |  |
| `author` | string | no |  |
| `binaryName` | string | no |  |
| `capabilities` | array or null | no | What the plugin offers, as a list of names. Same key and type as v1; read from the installed plugin.json by the MCP plugin. |
| `category` | string | yes |  |
| `cli` | string | no |  |
| `cliCommands` | array or null | no |  |
| `commands` | object or null | no | CLI surface mounted as `nself <command>`; null when the plugin adds no command. |
| `compat` | object or null | no |  |
| `consumes` | array or null | no |  |
| `dependencies` | array or null | no |  |
| `deprecation` | object or null | no |  |
| `description` | string | yes |  |
| `docs_url` | string | no | pattern `^https://` |
| `entryPoint` | string | no |  |
| `env` | object or null | no |  |
| `envVars` | array or null | no |  |
| `framework` | string | no |  |
| `graphql` | object or null | no |  |
| `health_endpoint` | string | no |  |
| `homepage` | string | no |  |
| `installable` | boolean or null | no | Defaults to true when absent. |
| `isCommercial` | boolean | no |  |
| `language` | string | no |  |
| `license` | string | yes | free or licensed. Bundle membership is not a manifest field (ADR 0008).; one of free, licensed |
| `licenseType` | string | no |  |
| `license_spdx` | string | no | Licence text carried from a v1 file (an SPDX identifier or Source-Available); shown by plugin info. |
| `manifest_version` | integer | yes | Always 2.; always 2 |
| `maturity` | string | yes | Lifecycle. deferred requires deprecation.state.; one of implemented, experimental, scaffolded, planned, deferred |
| `maxNselfVersion` | string | no |  |
| `migrations` | object or null | no |  |
| `minNodeVersion` | string | no |  |
| `minNselfVersion` | string | no |  |
| `multiApp` | object or null | no |  |
| `name` | string | yes | Plugin slug, unique per tier.; pattern `^[a-z][a-z0-9-]*$` |
| `optionalDependencies` | array or null | no |  |
| `packageManager` | string | no |  |
| `permissions` | array or object | no | Same key and shape as v1: canonical permission strings, or categories mapped to actions. |
| `platform_checksums` | object | no |  |
| `pluginType` | string | no |  |
| `port` | integer | no |  |
| `provides` | array or null | no |  |
| `repository` | string | no |  |
| `requiredEntitlements` | array or null | no |  |
| `requires` | object or null | no |  |
| `requires_license` | boolean | no |  |
| `rest_routes` | array or null | no |  |
| `routes` | array or null | no |  |
| `runtime` | string | no |  |
| `schema` | string or null | no | np_<name with - replaced by _>. |
| `seed` | object or null | no |  |
| `service` | object | yes | How the plugin runs. kind compose requires compose and healthcheck. |
| `status` | string | no |  |
| `systemDependencies` | object or null | no |  |
| `tables` | array or null | no |  |
| `tags` | array or null | no |  |
| `tier` | string | no |  |
| `updated_at` | string | no |  |
| `version` | string | yes | Plugin version (semver).; pattern `^v?[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$` |
| `views` | array or null | no |  |
| `visibility` | string | no |  |
| `webhooks` | array or null | no |  |

<!-- GENERATED:fields END -->

### commands

`commands` is `{command, binary, summary?, side_effect?, output?, json?, confirm?, surface?, subcommands: [...]}` or `null`. `command` and every space-separated segment of a subcommand `name` match `^[a-z][a-z0-9-]*$`; `binary` matches `^nself-[a-z][a-z0-9-]*$`. `command` must not be one of the core command verbs (E113). A subcommand has `name`, `summary`, `side_effect`, `output`, `json`, `args`, `flags`, and optional `confirm` and `surface`.

- `confirm: {flags: [...], plan?: {flag, id_flag}}` names the bool flags that confirm a destructive run. An empty `flags` list is valid (a nonce-only round trip). Every named flag must be a declared bool flag of the same command, and `plan.id_flag` a declared string flag (E106 names the field, for example `commands.subcommands[0].confirm.flags[0]`). The root declares no flags, so only an empty list is valid there.
- `surface: all | cli-only` (default `all`).
- An arg takes `secret: bool`; a flag takes `secret: bool` and `cli_only: bool`.

## Maturity and status

| v1 `status` | v2 `maturity` | compatibility `status` |
|---|---|---|
| `stable`, `beta`, absent | `implemented` | `stable` |
| `experimental`, `alpha` | `experimental` | `experimental` |
| `planned` | `planned` | `planned` |
| `deprecated` | `deferred` + `deprecation.state: deprecated` | `deprecated` |
| `eol` | `deferred` + `deprecation.state: eol` | `eol` |
| none | `scaffolded` | `experimental` |

`deferred` requires `deprecation.state`. State `deprecated` also requires `announcedDate`, `eolDate` and `migrationGuide`, because released CLIs reject a deprecated plugin without them.

## Compatibility keys

These keys exist only so released 1.4.x CLIs keep decoding a v2 file. `manifestv2migrate -compat` writes them; loading a file whose derived keys differ from the v2 fields fails with E112. E112 checks the derived keys only: the carried keys (`entryPoint`, `runtime`, `cli`, `cliCommands` entries other than the canonical command, and a licensed plugin's `tier` and `licenseType`) are trusted as written, so a hand edit there passes the gate.

| Key | Derived from |
|---|---|
| `pluginType`, `binaryName` | `commands` (`cli` and `commands.binary`; empty when `commands` is null) |
| `minNselfVersion` | lower bound of `requires.nself` |
| `status` | `maturity` and `deprecation.state` (table above) |
| `isCommercial`, `licenseType`, `requires_license`, `tier` | `license`: `free` gives `false, free, false, free`; `licensed` gives `true, <licenseType>, true, <tier>`, where the licensed plugin's own `licenseType` and `tier` (`max`, `cloud`, `internal`) are carried and `pro` is used when it states none |
| `cliCommands` | the entry named like `commands.command` takes `commands.summary` as its description; other entries are carried |
| `entryPoint`, `runtime`, `cli` | carried verbatim from v1: no v2 field defines them |

The v1 `license` text (an SPDX identifier or `Source-Available`) is kept in `license_spdx`; `nself plugin info` shows it as before. Released CLIs read the v2 `license` value (`free` or `licensed`) as the text.

v1 reading and all compatibility keys are removed at v1.6.0.

## Errors

| Code | Meaning |
|---|---|
| E106 | invalid manifest; the message names the field |
| E111 | a forbidden key in a v2 file |
| E112 | compatibility keys drifted from the v2 fields |
| E113 | `commands.command` is a core command verb |
| E114 | unsupported `manifest_version` |

## Migrating a v1 plugin

```bash
go run github.com/nself-org/cli/tools/manifestv2migrate -in plugin.json          # print canonical v2
go run github.com/nself-org/cli/tools/manifestv2migrate -in plugin.json -write   # convert in place
go run github.com/nself-org/cli/tools/manifestv2migrate -in plugin.json -compat -write   # regenerate compatibility keys
go run github.com/nself-org/cli/tools/manifestv2migrate -in plugin.json -check   # exit 1 unless canonical v2
go run github.com/nself-org/cli/tools/manifestv2migrate -in plugin.json -targets # binary and command pairs
```

Output has sorted keys, 2-space indent and a trailing newline; running it on its own output changes nothing. The converter maps v1 `binaryName` to `commands.binary`, the `cliCommands` entry named like it to `commands.command` (other binaries such as `sentry-server` or `billing` stay compatibility-only), and `binaryName: null` to `commands: null`. It derives `service` (a port means `compose`, a binary without a port means `cli`, otherwise `library`).

The converter never drops data silently. Every non-empty v1 key with no v2 home (for example `actions`, `config`, `binary_name`, `download_url`, `tarball`) is listed on stderr with its value path, and the run exits 1 with nothing written and nothing on stdout. This holds for the default run, `-write` and `-check`, and the lists are identical. Empty values (`null`, `""`, `[]`, `{}`) carry nothing and are ignored. Registry-owned keys (`bundles`, `checksum`, `tier_pair`, `author_public_key`, `signature`) are removed on purpose (ADR 0008) and reported as a note. A v1 file that v1.4.12 accepts without a `category` loads, but the converter will not write an invalid v2 file: add the `category` first.

Some v1 keys convert when their value converts without loss:

- `routes` (the plugin's own API: `method`, `path`, `auth`, optional `description` and `hmac`) becomes `rest_routes` (`description` becomes `summary`; `auth` and `hmac` are optional additions). It never becomes the v2 `routes` key, which is nginx exposure (`path`, `upstream_port`). An entry with any other key, or a file that already has `rest_routes`, stays refused.
- `capabilities` (a list of names) is an optional v2 key with the same name and type.
- `env` or `env_vars` becomes `env {required, optional}` only as `{required: [name], optional: [name]}`. Defaults, descriptions, per-variable flags and bare name lists have no place in `env`, so they stay refused.

`-drop <file>` is the one way to discard data. The file lists dead v1 keys, one per line with a `# evidence` comment; `tools/manifestv2migrate/dead-keys.txt` is the reviewed list (keys no code reads, not even a dev tool or build script). Only listed keys are dropped: a default or `-check` run reports what would be dropped, `-write` reports what was, and any key that is neither mapped nor listed still refuses. A mapped key cannot be listed.
