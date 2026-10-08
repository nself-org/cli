# nself add

<!-- BEGIN PROSE:summary -->
> Install a plugin or bundle.
<!-- END PROSE:summary -->

## Synopsis

```
nself add <name> [name...] [flags]
```

## Description

<!-- BEGIN PROSE:description -->
Install a plugin or bundle by name.

This is the short form of `nself add`, with bundle awareness: if
the name is a bundle (`nchat`, `nclaw`, `ntv`, `nfamily`, `clawde`, `nsentry`)
the whole bundle is installed, otherwise the name is resolved as a plugin.

Third-party plugins install by URL rather than by name:

  nself add https://example.com/my-plugin.tar.gz

Once installed, the plugin's commands are available directly:

  nself add waf
  nself waf status
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--allow-eol` | `false` | Allow installing an EOL plugin (not recommended) |
| `--channel` | `stable` | Release channel: stable \| beta \| canary |
| `--checksum` | `""` | Expected SHA-256 checksum of the downloaded archive (third-party URL installs only) |
| `--dry-run` | `false` | Show what would be installed without making changes |
| `--force` | `false` | Reinstall even when the plugin is already present |
| `--key` | `""` | License key for pro plugins |
| `--preview` | `false` | Preview the dependency tree without installing |
| `--show-graph` | `false` | Show dependency graph with topological sort order |
| `--skip-sbom-check` | `false` | Skip SBOM verification (air-gapped installs only — sets NSELF_SKIP_SBOM_CHECK=1) |
| `--strict` | `false` | Fail if any plugin in the bundle is missing from the registry |
| `--tier` | `""` | Force "free" or "pro" for a slug served as both (e.g. cron, notify); default resolves by license entitlement |
| `--version` | `""` | Install a specific version |
| `--with-optional` | `false` | Include optional dependencies in --preview output |
| `--yes` | `false` | Skip confirmation prompts (required for third-party URL installs in CI) |
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
nself add waf                  # one plugin
  nself add waf cdn analytics    # several at once
  nself add nchat                # a whole bundle
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[Commands]] — full command index
- [[Core-Services]] — what a stack is made of
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
