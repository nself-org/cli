# Plugin Install

## Install a Plugin

```bash
nself plugin install {name}
```

For free plugins, the CLI may prompt you to create a free account the first time:

```
Create a free account? [Y/n]
```

This generates a `nself_free_<uuid>` key stored on your device. The key enables install telemetry and conversion tracking. No email or payment is required. Declining the prompt still allows the install to proceed.

To persist your free key across sessions:

```bash
nself license set nself_free_<uuid>
```

After installing a few free plugins, the CLI displays a one-line prompt about ɳSelf+.

For licensed plugins, set your license key first, see [[Plugin-Licensing]].

Plugins marked `beta` install with a warning printed to stderr. Plugins marked `planned` are not yet available, the install command returns an error with a link to the release timeline. See [[Plugin-Status-Badges]] for details.

### Permission Validation

Every plugin manifest may declare a `permissions` array. The CLI validates each permission against the canonical allowlist at install time and rejects unknown permission strings (fail-closed). A structured audit log line is emitted after a successful install listing the granted permissions.

Plugins holding elevated permissions (`system:exec`, `network:internet`) print a visible warning to stderr. Use `nself plugin info <name>` to review the permission set before installing.

See [[Plugin-Permissions]] for the full permission catalog and tier roadmap.

After installing any plugin, regenerate your stack:

```bash
nself build
nself restart
```

## Remove a Plugin

```bash
nself plugin remove {name}
```

By default, the plugin's database schema (`np_{name}`) is dropped. To keep data:

```bash
nself plugin remove {name} --keep-data
```

## Update Plugins

```bash
# Update a single plugin
nself plugin update {name}

# Update all installed plugins (name omitted)
nself plugin update
```

## List Installed Plugins

```bash
nself plugin list
```

Shows all installed plugins with version and status.

## Get Plugin Info

```bash
nself plugin info {name}
```

Shows plugin description, version, port, tables, dependencies, and env vars.

## Check Available Plugins

```bash
# Browse the free registry
nself plugin search

# Search by keyword
nself plugin search {keyword}
```

## Install Multiple Plugins

```bash
nself plugin install chat livekit streaming
nself build
```

## Plugin Dependencies

Some plugins require others to be installed first. ɳSelf resolves dependencies automatically during install. If a required plugin isn't installed, ɳSelf installs it for you.

Example: `claw` requires `ai`. Installing `claw` automatically installs `ai` if missing.

## Environment Variables

`nself add` and `nself remove` load the project `.env` before choosing the plugin
directory. Set `NSELF_PLUGIN_DIR` there to use a project-specific directory; remove
also checks that directory's bundle ledger when deciding whether to remove a bundle.

For development installs with `NSELF_LICENSE_SKIP_VERIFY=1`, pass `--force` to
acknowledge the signature verification bypass. The bypass is refused in production
and staging even with `--force`.

Each plugin declares its required env vars in `plugin.json`. After install, add the required vars to your `.env` file:

```bash
nself plugin info {name}   # shows required env vars
```

Then run `nself build` to pick up the new configuration.

## Download Source

Free plugin tarballs are served via the `plugins.nself.org` registry worker, which redirects
to Cloudflare R2 as the primary CDN (free global egress). GitHub Releases is the automatic
fallback when R2 is unavailable. Both mirrors hold identical content and SHA-256 checksums are
verified between them on every release. The CLI follows the 302 redirect transparently.

To override the registry URL for private testing:

```bash
export NSELF_PLUGIN_REGISTRY=https://your-registry-mirror.example.com
nself plugin install {name}
```

## Related Pages

- [[Plugin-Overview]], What plugins are and pricing tiers
- [[Plugin-Licensing]], License keys for licensed plugins
- [[Plugin-Status-Badges]], stable, beta, and planned behavior at install time
- [[Plugin-Dev-Guide]], Build your own plugin
