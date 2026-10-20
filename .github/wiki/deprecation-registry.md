# Deprecation Registry

This page lists all CLI commands, flags, and env vars that are currently in Phase 1 or Phase 2
of the deprecation cycle.

## Global Deprecation Flags

The following persistent flags are available on all `nself` commands:

| Flag | Description |
|------|-------------|
| `--no-deprecation-warnings` | Suppress all deprecation warnings (use in scripts to avoid noisy stderr) |

Deprecation warnings are always written to **stderr**, never stdout, so they never break piped output.
Use `--no-deprecation-warnings` only in scripted environments where the warning is expected and irrelevant.

## Currently Deprecated Items

### v1.0.9 (April 2026)

| Item | Type | Since | Replacement | Phase | EOL |
|------|------|-------|-------------|-------|-----|
| `nMedia` bundle | bundle name | v1.0.9 | `nTV` bundle | 1 | v1.2.0 |
| `nself_sessions.user_agent` (Hasura) | schema column | v1.0.9 | `device_type` + `device_name` | 1 | 2027-04-17 |

No CLI commands or flags are deprecated at v1.0.9.

## Break-outs and removed commands (v1.5)

In v1.5 mode these core commands left the binary for plugins. The old spelling still works: the first time you
type it without the plugin installed, nSelf installs the plugin through the verified installer (free tier,
checksum and release signature checked, configured registry only), prints one line on stderr and runs it.

| Old command | Now | Plugin |
|-------------|-----|--------|
| `nself ci` | `nself ci` | `ci` |
| `nself runner` | `nself ci nodes` | `ci` |
| `nself functions` | `nself functions` | `functions` |
| `nself server` | `nself infra server` | `infra` |
| `nself plugin init`, `new`, `dev`, `debug`, `link`, `unlink`, `test` | `nself plugin-dev <same>` | `plugin-dev` |
| `nself plugin marketplace`, `nself plugin submit` | removed (E410): see https://nself.org/plugins | none |

The line printed once per process: `warning: 'nself <old>' now comes from the <slug> plugin; installed <slug>@<version>`.
`--quiet` and `--no-deprecation-warnings` silence it.

- **Version.** The version in `nself.lock.yaml` (`plugins.<slug>.version`) when the project pins one, else the catalogue's current release.
- **When it cannot install.** Offline, refused, unsigned or tampered releases, and `NSELF_NO_AUTO_ADD=1`, exit 1 with
  `[E404]` and the fix `nself add <slug>`. With `--json`, stdout carries `.error.code == "E404"` and the same remediation.
  Export `NSELF_NO_AUTO_ADD=1` (or `true`, any case) in the shell, before running nself; project `.env` files do not set this bootstrap policy. Use it in an environment that must never download code (a gate on a locked-down runner), and
  run `nself add ci` there once.
- **A plugin you disabled** (`.disabled` in its plugin directory) is never reinstalled: the command fails with `[E407]`.
- **v1.4 mode is unchanged**: the commands stay in core, and nothing is installed.

## Deprecation Cycle

ɳSelf uses a 3-phase deprecation cycle across all surfaces:

**Phase 1, Warning:** Item works. Every invocation prints:
```
[DEPRECATED] 'nself old-cmd' (since v1.0.9) → use 'nself new-cmd'. Docs: https://nself.org/docs/...
```

**Phase 2, Deprecated:** Item still works. Warning includes removal timeline.

**Phase 3, Removed:** Item gone. Clear error with migration guide link.

Emergency removals (security CVEs only) skip phases. See the full doctrine at
`nself.org/docs/deprecation-cycle`.

## Adding a Deprecation

To deprecate a CLI command or flag:

1. Add an entry to `cli/internal/deprecation/registry.yaml`.
2. Open a PR, the `deprecation-check.yml` CI gate verifies the entry is present.
3. Update this page.

```yaml
# registry.yaml
items:
  - name: "nself old-cmd"
    type: command
    since: "v1.1.0"
    replacement: "nself new-cmd"
    docs_url: "https://nself.org/docs/migrate/v1-1-0"
    phase: 1
```

[[Home]]
