# Migration Roadmap

This page covers the three-version migration path: v0.9 → v1.0.9 → v1.1.0.

---

## v0.9 → v1.0.9

See [[Upgrade-From-v0.9]] for the full v0.9 upgrade guide.

Quick summary:

- CLI is now a Go binary (no shell wrapper). Run `brew install nself-org/tap/nself`.
- New project init: `nself init` replaces the old `nself-setup.sh` script.
- Plugin system overhauled. Re-install all plugins after upgrade.
- All v0.9 env vars still accepted (deprecated, see [[Migration-Signals-v1.0.9]]).

---

## v1.0.8 → v1.0.9

See [[Migration-Signals-v1.0.9]] for the complete signal table.

Quick summary:

- Non-breaking. All commands stable.
- Bundle renamed: `nMedia` → `nTV`. Re-install: `nself plugin install nTV`.
- Plugin SDK Go module path changed. Plugin authors update `go.mod`.
- Hasura `nself_sessions.user_agent` column deprecated (EOL 2027-04-17).

Upgrade:

```bash
brew upgrade nself
nself version  # verify: v1.0.9
nself doctor   # all checks should pass
```

---

## v1.0.9 → v1.1.0 (PLANNED, subject to change)

> These are anticipated signals based on the v1.1.0 scope (S31). Details may change
> before v1.1.0 ships. Check release notes for the definitive list.

### Anticipated additions (non-breaking)

| Signal type | Detail |
|-------------|--------|
| New command | `nself ai agents` subcommand , AI agent orchestration (additive) |
| New flag | `--watch` on `nself build` , live-rebuild on config change (additive) |
| New bundle | `nFamily` bundle , `nself plugin install nFamily` |
| New env var | `NSELF_AI_AGENT_TIMEOUT` , optional, has default |
| New API endpoint | `POST /plugins/batch` on Marketplace Worker (additive) |

### Anticipated bundle changes

| Change | Old | New | Action |
|--------|-----|-----|--------|
| ClawDE+ bundle goes live | "Free Beta" label | `$1.99/mo` | No plugin re-install needed; license key required to retain server sync |
| ɳFamily bundle added | — | ɳFamily plugins | `nself plugin install nFamily` when available |

### Anticipated deprecations entering Phase 1 (v1.1.0)

No CLI commands are currently planned for deprecation in v1.1.0.
Any deprecations will follow the deprecation cycle doctrine (`.claude/docs/doctrines/deprecation-cycle.md`)
3-phase policy and will emit warnings before removal.

---

## Deprecation Cycle Policy

ɳSelf follows a 3-phase deprecation cycle for all API surfaces:

1. **Phase 1, Warning** (1 minor version): item works, warning emitted on use.
2. **Phase 2, Deprecated** (1 minor version): item works, stronger warning + removal notice.
3. **Phase 3, Removed**: item gone, clear error with migration guide.

Emergency removals (security CVEs only) can skip phases. See the full
doctrine at `.claude/docs/doctrines/deprecation-cycle.md`.

---

## Related Pages

- [[Upgrade-From-v0.9]], full v0.9 → v1.0.x upgrade path
- [[Migration-Signals-v1.0.9]], v1.0.8 → v1.0.9 signal table
- [[Changelog]], full release notes per version

[[Home]]
