# nself plugin

<!-- BEGIN PROSE:summary -->
> Install, remove, update, and manage ɳSelf plugins.
<!-- END PROSE:summary -->

## Synopsis

```
nself plugin <subcommand> [flags]
```

## Description

<!-- BEGIN PROSE:description -->
`nself plugin` manages the ɳSelf plugin ecosystem. Plugins extend the CLI and your backend stack with new capabilities. Free plugins (MIT licensed) install without a key. Pro plugins require a valid membership license key, set one with `nself license set`.

When you install a plugin, ɳSelf checks your license tier against the plugin's requirements, downloads the plugin binary and Docker image, registers the plugin with the stack, and prepares database migrations. Run `nself build` and `nself restart` after installing plugins to include them in the generated `docker-compose.yml`.

Unknown subcommands are proxied to the matching plugin binary: `nself plugin ai <action>` calls `nself-ai <action>`. This allows installed plugins to expose their own subcommands through the ɳSelf CLI namespace.

## Plugin Status Badges

Plugins carry a status field in the registry. The `list` subcommand shows badges for non-stable plugins:

```
ai                  [installed]
browser             [beta]
nfamily             [planned]
```

Install behavior by status:

- **stable**, installs without warnings (default for most plugins)
- **experimental**, prints a warning to stderr, then installs
- **beta**, prints a warning to stderr, then installs
- **deprecated**, prints a deprecation warning with EOL date and migration guide, then installs
- **eol**, install is blocked; use `--allow-eol` to override (not recommended)
- **planned**, install is rejected with a "coming soon" message and a link to the release timeline

EOL plugins are hidden from `nself plugin list` by default. Use `--show-eol` to include them.

See [[Plugin-Status-Badges]] for the full reference.

## Install telemetry

After a successful `nself plugin install`, the CLI sends a single fire-and-forget event to `plugins.nself.org/plugins/:name/install-event`. This increments the public download counter shown in the plugin marketplace.

The event body contains one field: `instanceId`, which is an opaque SHA-256 hash of a machine-local identifier. No hostname, IP address, username, or project name is transmitted. The event is deduplicated per (instance, plugin) per ISO week, so reinstalling the same plugin in the same week does not double-count. If the network is unavailable, the event is silently dropped with no retry.

To opt out, set `NSELF_DISABLE_TELEMETRY=1` in your environment or `.env.local`.

## A slug served as both free and pro

A small number of plugins (`cron`, `notify`) ship as a genuine free/pro pair: the same product, listed twice in the registry under one slug. `nself plugin install cron` resolves which entry to install like this:

- If your license entitles the bundle the pro entry belongs to (checked via the same bundle-entitlement path `nself bundle install` uses), you get **pro**.
- Otherwise, or with no license key configured, you get **free**. Free never requires a key.
- Pass `--tier free` or `--tier pro` to force a side. `--tier free` always succeeds. `--tier pro` still runs the entitlement check — it is a way to *ask* for pro, not a way to bypass licensing — and fails with a clear "buy the bundle" error if you're not entitled.

Run `nself plugin list --available` to see every tier of every such slug side by side, with the tier a plain `nself plugin install <name>` would resolve to today marked in the `Default` column.

Any *other* slug collision — two unrelated registry entries that happen to share a name, not a declared tier pair — is refused outright with an error naming both entries. The CLI never silently installs "whichever one came first" for an ambiguous slug.

## Official by name, third-party by URL

`nself plugin install <name>` resolves against the official plugins.nself.org registry: license-checked for pro tiers, and its tarball is Ed25519-signature-verified against a registry-pinned author key.

`nself plugin install <https-url>` installs directly from an arbitrary URL instead, for plugins that aren't in the registry. This path:

- Never contacts the official registry — no license check, no EOL/deprecation lifecycle handling, no install telemetry.
- Is **not** signature-verified. There is no registry-pinned key to check an arbitrary URL's tarball against.
- Verifies a checksum only when you pass `--checksum <sha256>` yourself (obtained out-of-band, e.g. from the plugin's README or release notes) — the same trust model as `pip install --hash=` or a Homebrew formula. Without `--checksum`, the download's integrity is unverified and the CLI says so.
- Always prints a warning naming the source host and asks for interactive confirmation before downloading anything, unless you pass `--yes` (for CI/non-interactive use).
- Requires `https://` (plain `http://` is rejected, except against `localhost`/`127.0.0.1` for local development).

`nself plugin outdated` and `nself plugin list --detailed` skip plugins that came from a third-party URL when comparing against the registry — there's nothing registered to compare them to.

## Freshness

`nself plugin list --detailed` prints an `Updated` column per plugin, sourced from the registry entry's (or local `plugin.json`'s) `updated_at` field when present. As of the current plugins.nself.org registry, no plugin carries a per-plugin `updated_at` — only a registry-wide snapshot timestamp, which `--detailed` prints as a `Registry snapshot: <timestamp>` header line instead of a per-row value. The column is plumbing ready for the day the registry starts sending a per-plugin value; it never fabricates one.

`nself plugin outdated` compares every installed plugin's version against the registry and lists the ones behind, with `--json` for scripting. It exits `0` when everything is current and `1` when at least one plugin is outdated, so it composes in CI.

With no plugins installed it answers from local state and never contacts the registry, so it stays fast and works offline. The registry is only consulted once there is something to compare.
## Permissions

`nself plugin info <name>` lists the permissions a plugin declares, and every
one of them is checked against a fixed allowlist before the plugin installs. An
unrecognised permission blocks the install; the check is fail-closed on purpose.

Manifests declare permissions in one of two forms. The canonical form is a flat
list of the allowlist's own vocabulary:

```json
"permissions": ["db:read", "network:internet", "fs:write:uploads"]
```

Most published plugins use a more specific descriptive form instead, naming the
hosts, paths and operations rather than the categories:

```json
"permissions": {
  "database": ["create", "read"],
  "network": ["api.stripe.com"],
  "filesystem": ["logs"]
}
```

Both are enforced. The descriptive form is reduced to the canonical vocabulary
before validation, and **the reduction always widens**: a plugin naming one host
is enforced as general internet access, and any database verb that is not
plainly a read is enforced as a write. So `nself plugin info` prints what a
declaration is *enforced as*, not only what it says:

```
Permissions:
  database:create
  network:api.stripe.com

  Enforced as:
    db:write
    network:internet
```

The gap between the two lines is the point. A declaration that looks narrow is
enforced broadly, because under-stating a permission is the one direction a
fail-closed check must never take.
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Subcommands

<!-- BEGIN GENERATED:subcommands -->
| Name | Description |
|------|-------------|
| `audit-tables` | Audit np_* table row counts and multi-tenant isolation compliance |
| `compat-check` | Check installed plugins against the current CLI version |
| `count` | Show the authoritative plugin counts (free, pro, advertised) |
| `debug` | Attach a dlv debugger to a running plugin process |
| `dev` | Start a plugin in development mode with hot-reload |
| `disable` | Disable a plugin (excluded from compose on next build) |
| `enable` | Re-enable a previously disabled plugin |
| `info` | Show detailed plugin information |
| `init` | Scaffold a new plugin project |
| `install` | Install one or more plugins (license check for pro); a plugin arg may be a name or an https:// URL |
| `inventory` | List installed plugins with version, tier, and status |
| `link` | Register a local plugin directory as a shadow override |
| `list` | List available and installed plugins |
| `logs` | Tail logs from a plugin container |
| `marketplace` | Browse the ɳSelf plugin marketplace |
| `new` | Scaffold a new plugin project (deprecated: use 'init') |
| `outdated` | List installed plugins with a newer version available |
| `refresh` | Force refresh the registry cache |
| `remove` | Remove a plugin |
| `search` | Search plugins by name, description, or tag |
| `start` | Start a plugin service |
| `status` | Show plugin status |
| `stop` | Stop a plugin service |
| `submit` | Validate a plugin for submission to the registry |
| `test` | Run a plugin's test suite (unit + smoke install/uninstall) |
| `unlink` | Remove a local plugin shadow, restoring the registry version |
| `update` | Update a specific plugin or all plugins |
| `updates` | Check for available plugin updates |
<!-- END GENERATED:subcommands -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# List all available plugins
nself plugin list

# List only installed plugins
nself plugin list --installed

# Filter by category
nself plugin list --category ai

# Install a free plugin
nself plugin install notify

# Install multiple plugins in one command
nself plugin install ai claw mux

# Install a pro plugin (uses saved license key)
nself plugin install ai

# Install a pro plugin with an inline key
nself plugin install livekit --key nself_pro_xxxxx...

# Install a specific version
nself plugin install recording --version 1.2.0

# See both tiers of a slug served twice (e.g. cron: free + pro), with the
# resolved default marked
nself plugin list --available

# Force the free entry of a free/pro slug, ignoring entitlement
nself plugin install cron --tier free

# Ask for the pro entry explicitly (still requires an entitled license)
nself plugin install cron --tier pro

# Remove a plugin
nself plugin remove ai

# Remove a plugin but keep its database data
nself plugin remove livekit --keep-data

# Force remove (ignores dependents)
nself plugin remove recording --force

# Update a specific plugin
nself plugin update ai

# Update all plugins
nself plugin update

# Check for available updates without installing
nself plugin updates

# List installed plugins that are behind the registry version (exits 1 if any are)
nself plugin outdated

# Same, as JSON for scripting
nself plugin outdated --json

# Show freshness (Updated column + registry snapshot timestamp)
nself plugin list --detailed

# Install a third-party plugin directly from a URL (prompts for confirmation)
nself plugin install https://example.com/releases/my-plugin-1.0.0.tar.gz

# Same, verifying against a checksum you obtained from the source yourself
nself plugin install https://example.com/releases/my-plugin-1.0.0.tar.gz --checksum <sha256>

# Same, non-interactively for CI (still prints the third-party warning)
nself plugin install https://example.com/releases/my-plugin-1.0.0.tar.gz --yes

# Show installed plugins with version and tier
nself plugin inventory

# Refresh registry cache
nself plugin refresh

# Start/stop a plugin service
nself plugin start ai
nself plugin stop ai

# Show plugin status
nself plugin status
nself plugin status ai --detailed

# Check compatibility after a CLI upgrade
nself plugin compat-check

# List all plugins including EOL ones
nself plugin list --show-eol

# Install an EOL plugin (not recommended — use only when a replacement is unavailable)
nself plugin install old-plugin --allow-eol
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[cmd-plugin-compat-check]] — compatibility check reference
- [[cmd-plugin-dev]] — plugin author dev mode
- [[cmd-plugin-link]] — link a local plugin directory into the stack
- [[cmd-plugin-unlink]] — remove a plugin from the linked set
- [[cmd-plugin-test]] — run unit and smoke tests
- [[cmd-plugin-debug]] — attach a Delve debugger
- [[cmd-plugin-logs]] — tail plugin container logs
- [[cmd-plugin-marketplace]] — browse the marketplace
- [[Plugin-Status-Badges]] — lifecycle status reference
- [[Plugin-Licensing]] — license tiers and key format
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
