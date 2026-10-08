# Plugin Architecture

ɳSelf's plugin system extends the base stack with additional services, communication tools, AI engines, media pipelines, commerce systems, and more. Free plugins are MIT licensed and install without any key. Pro plugins require a valid license key, but otherwise follow exactly the same architecture. The install pipeline, schema isolation, compose overlay, and Nginx route injection work identically for both tiers; the only difference is a license check gate before the download proceeds.

---

## Plugin Manifest

Every plugin ships with a `plugin.json` manifest at its root. This manifest is the single source of truth for what the plugin needs: ports, database tables, environment variables, dependencies, and health check information. The CLI reads this manifest at install time, at build time, and whenever it generates or validates configuration.

| Field | Type | Purpose |
|-------|------|---------|
| `name` | string | Unique plugin ID (e.g., `"chat"`) |
| `version` | semver | e.g., `"1.0.0"` |
| `description` | string | What the plugin does |
| `category` | string | `communication`, `media`, `commerce`, etc. |
| `license` | string | `"MIT"` (free) or `"Source-Available"` (pro) |
| `port` | int | Network port the plugin service runs on |
| `language` | string | `go`, `rust`, `typescript`, `python` |
| `tables` | string[] | Postgres tables the plugin owns (prefixed `np_{name}_`) |
| `envVars` | object[] | Required and optional environment variables |
| `dependencies` | string[] | Other plugins this plugin requires |
| `health_endpoint` | string | Health check path (e.g., `"/health"`) |

A minimal manifest looks like this:

```json
{
  "name": "chat",
  "version": "1.0.0",
  "description": "Real-time messaging service backed by Postgres and WebSockets",
  "category": "communication",
  "license": "MIT",
  "port": 3401,
  "language": "go",
  "tables": ["rooms", "messages", "members"],
  "envVars": [
    { "key": "CHAT_MAX_MESSAGE_SIZE", "default": "4096", "required": false }
  ],
  "dependencies": [],
  "health_endpoint": "/health"
}
```

---

## Schema Isolation

Every plugin gets its own Postgres schema. This ensures plugins never collide with each other or with the base ɳSelf schema, and makes it trivial to identify which objects belong to which plugin.

The naming conventions are strict and enforced by the CLI:

- **Schema:** `np_{plugin_name}` (e.g., `np_chat`)
- **Role:** `np_{plugin_name}_role` (e.g., `np_chat_role`)
- **Tables:** `np_{plugin_name}_{table}` (e.g., `np_chat_messages`, `np_chat_rooms`)
- **Version tracking:** all plugin schema versions are recorded in `np_common.schema_versions`

Schema creation is idempotent, the CLI uses `CREATE SCHEMA IF NOT EXISTS` and migration guards throughout, so it is always safe to re-run. This means `nself plugin install` can be repeated without corrupting existing data, and upgrades apply only the missing migration steps.

Hasura automatically tracks tables in `np_*` schemas so plugin data is immediately queryable through the GraphQL API without manual configuration.

---

## Compose Overlay Injection

When a plugin is installed, it ships a `docker-compose.plugin.yml` overlay file alongside its other assets. During `nself build`, the CLI merges all installed plugin overlays into the generated `docker-compose.yml` using a deep-merge strategy.

Plugins can contribute:

- **New service definitions**, the primary plugin container and any sidecar processes it needs
- **New named volumes**, persistent storage scoped to the plugin
- **New network connections**, attaching the plugin service to the shared `nself` bridge network

Plugins **cannot** remove, rename, or override existing base services. The merge is additive only. If a plugin overlay attempts to redefine a service that already exists in the base configuration, the build step rejects it with a validation error.

**Example:** Running `nself plugin install chat` adds a `chat` service running on port 3401 to the generated compose file:

```yaml
# docker-compose.plugin.yml (chat plugin)
services:
  chat:
    image: nself/plugin-chat:1.0.0
    restart: unless-stopped
    ports:
      - "127.0.0.1:3401:3401"
    environment:
      - CHAT_MAX_MESSAGE_SIZE=${CHAT_MAX_MESSAGE_SIZE}
    networks:
      - nself
    depends_on:
      - postgres
```

---

## Nginx Route Injection

Plugins declare their Nginx routes in the manifest. During `nself build`, the CLI writes a dedicated configuration file for each installed plugin at `nginx/routes/plugin-{name}.conf`. Routes from different plugins never share a file, so removing a plugin cleanly removes its routes without touching anything else.

Plugins can declare three kinds of routes:

- **Subdomain routes**, map `chat.{BASE_DOMAIN}` to the plugin service running on its declared port (e.g., the chat plugin proxies `chat.example.com` → `127.0.0.1:3401`)
- **Webhook endpoints**, available at `webhooks.{BASE_DOMAIN}/{plugin-name}` for inbound HTTP callbacks from third-party services
- **Custom domain routes**, if `PLUGIN_{NAME}_WEBHOOK_DOMAIN` is set in the environment, the plugin can serve traffic on that custom domain instead of the default subdomain pattern

All plugin routes are managed entirely through `nself build`. Never hand-edit files under `nginx/routes/`, they are regenerated on every build and manual changes will be overwritten.

---

## Config Templating

Plugins declare their required and optional environment variables in the manifest `envVars` array. During `nself plugin install`, the CLI processes each declared variable:

1. **Required vars with no default**, the CLI prompts the user to enter a value interactively. The install will not proceed until all required vars are satisfied.
2. **Optional vars with defaults**, written to `.env.dev` automatically without prompting.
3. **All plugin env vars**, written to a plugin-scoped env file at `~/.nself/plugins/{name}/.env` with permissions `600`. This file is mounted into the plugin container at runtime.

This means plugin configuration is always traceable: every value either came from user input at install time or from a manifest default. There are no hidden side-effects.

---

## License Validation Flow

For Pro plugins, the CLI runs a license check before any download occurs. The flow is:

```
nself plugin install ai
  ├── 1. Check plugin list — "ai" is in the pro list
  ├── 2. Read license key from NSELF_PLUGIN_LICENSE_KEY or ~/.nself/license/key
  ├── 3. Validate format: must start with nself_pro_, nself_max_, nself_ent_, or nself_owner_
  ├── 4. Check local cache (~/.nself/license/cache, 24h TTL)
  ├── 5. If cache miss: POST https://ping.nself.org/license/validate
  │       Body: {"license_key": "...", "product": "plugins-pro"}
  │       200 → valid, cache result
  │       401/403/404 → invalid, deny
  │       Network error → fail open with warning
  └── 6. If valid: proceed with download + install
```

The cache has a 24-hour TTL so repeated installs in a session do not hit the network every time. On a network error, the CLI fails open with a warning rather than blocking the install, this is intentional so that air-gapped or offline environments are not broken by transient connectivity issues.

License keys are stored at `~/.nself/license/key` with `chmod 600`. Set your key with:

```bash
nself license set nself_pro_xxxxxxxx...
```

The key format encodes the tier in its prefix. Tier enforcement is server-side, the `ping.nself.org/license/validate` endpoint determines which plugins a given key is permitted to install.

---

## Installation Flow

The complete install sequence for any plugin:

```
nself plugin install <name>
  1. Fetch registry (with cache + GitHub fallback)
  2. License check (pro only — see above)
  3. Download tarball → verify SHA256 checksum
  4. Extract to ~/.nself/plugins/{name}/
  5. Install system dependencies (apt/brew/yum)
  6. Create Postgres schema (np_{name})
  7. Resolve plugin dependencies (recursive)
  8. Generate ~/.nself/plugins/{name}/.env
```

Dependency resolution in step 7 is recursive and cycle-safe. If plugin A depends on plugin B, and plugin B is not yet installed, the CLI installs B first (running its own full install sequence), then resumes installing A. Circular dependencies are detected and rejected at validation time before any download begins.

After installation, run `nself build` to regenerate the compose file and Nginx configuration with the new plugin included. Then `nself restart` to apply the changes.

---

## Uninstall

Removing a plugin comes in two forms depending on whether you want to keep the data:

```bash
nself plugin remove <name>              # Remove plugin + drop schema (destroys data)
nself plugin remove <name> --keep-data  # Remove plugin, keep schema and all data intact
```

The `--keep-data` flag is the safer default for production use, it pulls the plugin service out of compose and Nginx while preserving everything in Postgres. You can reinstall the plugin later and it will resume with the existing data.

After removing a plugin, run `nself build` and `nself restart` to apply the changes.

---

## Plugin command mount (contract v1)

`contract:cli.plugin-command-mount v1` (EPIC P7-CANON D6/D12/D15/D19/D20/D21): an installed, enabled plugin whose manifest declares a `commands` block is mounted as a real `nself <command> …` subtree, so plugin commands show up in help, `help --json`, the command registry and every surface generated from it.

- **Declaration** (manifest v2, owned by the plugin contract): `commands: {command, binary, subcommands: [{name, summary, side_effect, output, json, args, flags, confirm, surface}]}`. `command` matches `^[a-z][a-z0-9-]*$` and is not one of the ADR 0016 core verbs; `binary` matches `^nself-[a-z][a-z0-9-]*$`; a subcommand `name` is one or more space-separated segments, each matching the segment pattern (nested paths such as `server list`). Optional root fields `summary`, `side_effect`, `output`, `json` default to `side_effect: destructive` and `json: none` when absent. A v1 manifest (`pluginType: cli` with `binaryName`/`cliCommands`) normalises to the same block and mounts the same subtree.
- **Discovery** (`internal/plugin/mount.Discover(pluginDir, verbs) ([]Spec, []Problem)`): for each `<pluginDir>/<slug>/plugin.json`, skip plugins with a `.disabled` marker and manifests with no `commands` block. Manifest reads are capped at 1 MiB. The resolved binary `filepath.EvalSymlinks(<pluginDir>/bin/<binary>)` must exist, be executable and lie inside `<pluginDir>` after symlink resolution; anything else is a Problem E406 and the plugin is not mounted. The core verbs come in as a parameter from the generated canon table — never from `canon.Load` — so the mount adds no YAML parsing to startup. Discovery reads disk only, never the network, and reports specs sorted by slug.
- **Manifest reading**: discovery goes through `manifestv2` (the one manifest reader) and uses its quiet parse. `manifestv2.Load` prints a once-per-process v1 deprecation notice in v1.5 mode; mounting runs on every invocation, so the quiet parse keeps a mounted run byte-identical to the old proxy run, in both modes. A manifest the loader rejects (for example E113 for a core-verb `command`) is not mounted and becomes a Problem carrying the loader's code.
- **Builtin source**: `builtinFamily{slug, build}` values listed in `cmd/commands/builtin_families.go` (the composition root; the admin family, ADR 0022). A builtin family is mounted unless `<pluginDir>/<slug>/.disabled` exists (D19), and its commands are real cobra commands that keep root's pre-run hooks.
- **Mount** (`cmd/commands/plugin_mount.go`): per Spec a cobra subtree is added before the invocation decorator — root `Use: <command>`, one nested node per subcommand segment, every node `DisableFlagParsing: true`, annotations `nself.plugin=<slug>`, `nself.mount.source=installed|builtin`, `nself.side_effect`, `nself.output`, `nself.json`, `nself.args`, `nself.flags`, and (additive, D21) `nself.confirm` (compact JSON, absent when null) and `nself.surface`. GroupID `plugins` ("Plugin Commands:", added to root when at least one mount exists). Installed RunE execs the discovered `Spec.BinaryPath` after a fresh containment check. The unknown-command proxy uses the same check. Both execute the real path directly, preserving `$0` and leaving no temporary links or inherited file descriptor. The directory owner can still replace the file between check and exec; that owner already controls plugin binaries. Argv remains verbatim after `stripRootPersistentFlags` (which keeps `--json`), with no shell, inherited stdout/stderr and the plugin's exit status passed through.
- **Pre-run isolation (D20)**: the old proxy ran the plugin before cobra, so root `PersistentPreRunE` (OTel init, deprecation warning, command log, `--version`, source guard, monorepo chdir and notice, licence migration) never applied. Each installed mount root therefore sets its own no-op `PersistentPreRunE` and `PersistentPostRunE` (cobra runs the nearest), in both modes; the invocation decorator also skips installed bodies, so they take no project operation lock or lock token. A mounted plugin sees the same cwd, stdout, stderr and exit code as before. Builtin families keep root's pre-run.
- **An invocation that starts with a flag keeps today's behaviour**: when `os.Args[1]` is a flag (for example `nself --no-deprecation-warnings claw run x`), the old intercept never proxied — cobra answered with its unknown-command error. Mounting is skipped for those invocations, so their output and exit code stay byte-identical to the base binary in both modes. The mount itself is additive new surface for everything else, ungated in v1.4 per ADR 0021.
- **Collisions**: a `command` equal to a core verb, any cobra-native or builtin-family command or alias in the current tree (help, completion, man, version included), a break-out owned by another plugin, or another Spec's command (both lose) is not mounted; each collision prints exactly one stderr warning per process naming both owners (E405, unless `--quiet`/`--no-deprecation-warnings`) and is a `doctor` finding (the `plugin-mount` check lists every Problem). Warnings and doctor rows escape control characters in names and messages. A command that equals a core command scheduled as a break-out to the same slug (the generated breakouts table) is skipped silently in v1.4 mode: core wins, and core already proxies.
- **Registry**: an annotated node gets `canon: plugin` and the additive field `plugin: <slug>`; installed-source attributes come from the annotations with the defaults above, builtin-source attributes from the family's canon fragment entry (which must say `canon: plugin`). The completeness check does not require a canon fragment for installed mounts, and validation still rejects `canon: plugin` on non-annotated (cobra-native) entries. `counts.plugin` is additive; `counts.top_level` and the command inventory exclude `canon: plugin` commands.
- **Hermetic generation**: generators never call the installed mount — the committed registry and inventory never depend on the machine's plugin directory. Builtin families are always mounted, so the committed registry will contain `admin` and no installed plugin.
- **Not installed**: `nself <x>` keeps today's path — a relocation hint, or the proxy of a bare binary in the bin dir (v1 multi-binary plugins such as `sentry-server` keep working), or the install hint, now `nself add <x>`.

---

**See also:** [[Architecture]] | [[Compose-Generation]] | [[Nginx-Generation]] | [[Home]]
