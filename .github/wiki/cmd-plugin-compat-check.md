# nself plugin compat-check

> Check installed plugins for CLI version compatibility.

## Synopsis

```
nself plugin compat-check [flags]
```

## Description

`nself plugin compat-check` scans every installed plugin and verifies that the running CLI version falls within the plugin's declared compatibility range (`minNselfVersion` and `maxNselfVersion` fields in `plugin.json`).

Use this command after upgrading the CLI (`nself self-update`) to catch any installed plugins that declare an upper-bound cap on the CLI version they support. Incompatible plugins are listed with install-method-aware hints showing the exact command to reinstall or remove them.

The command exits with code 0 when all plugins are compatible and code 1 when at least one incompatibility is found. This makes it suitable for use in CI or deployment scripts.

## Flags

This command has no flags.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All installed plugins are compatible with the current CLI version |
| 1 | One or more plugins are incompatible |

## Examples

Check compatibility after a CLI upgrade:

```bash
nself self-update
nself plugin compat-check
```

Use in a CI step to gate a deploy:

```bash
nself plugin compat-check || { echo "Incompatible plugins found — resolve before deploy"; exit 1; }
```

Example output when all plugins are compatible:

```
All 12 installed plugins are compatible with nself vX.Y.Z.
```

Example output when an incompatibility is found:

```
Incompatible plugins (1):

  claw  — maxNselfVersion "1.0.8" is below current CLI v1.1.1
    Reinstall: nself plugin install claw

Run 'nself plugin install <name>' for each plugin listed above.
```

## How it works

For each plugin directory found in the plugin install path, `nself plugin compat-check` reads `plugin.json` and compares the `minNselfVersion` and `maxNselfVersion` fields against the running CLI version using semver ordering.

- If `minNselfVersion` is set and the CLI is below it, the plugin is incompatible.
- If `maxNselfVersion` is set and the CLI is above it, the plugin is incompatible.
- If neither field is set, the plugin is treated as compatible.

Install-method hints (Homebrew, direct, manual) are inferred from the install path so the suggested reinstall command matches how the plugin was originally installed.

## Relation to the plugin lifecycle

Plugins that have reached end-of-life (`status: eol`) are also flagged if they declare a `maxNselfVersion` that is exceeded. Use `nself plugin list --show-eol` to see all EOL plugins and `nself plugin remove <name>` to uninstall them.

## See also

- [[cmd-plugin]], full plugin management reference
- [[cmd-plugin]], list installed plugins with status badges
- [[Plugin-Status-Badges]], full compatibility matrix

← [[Commands]] | [[Home]] →
