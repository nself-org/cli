# Migration Signals: v1.0.8 to v1.0.9

This page documents every signal that could affect users upgrading from v1.0.8 to v1.0.9.

For users upgrading from v0.9, see [[Upgrade-From-v0.9]] first, then return here.

---

## Summary

v1.0.9 is a **non-breaking long-term support release**. No CLI commands were renamed or removed.
No env vars were deleted. All 47 commands remain stable.

This page documents the audit explicitly so the absence of migration signals is itself a documented finding.

---

## Signal Table

| Signal type | Old value | New value | Since | Migration action | Deprecation warning |
|-------------|-----------|-----------|-------|-----------------|---------------------|
| CLI commands | (audited , 47 commands, none changed) | — | v1.0.9 | No action needed | — |
| CLI flags | (audited , 12 flags, none removed) | — | v1.0.9 | No action needed | — |
| Environment variables | (audited , 200+ vars, none removed) | — | v1.0.9 | No action needed | — |
| Plugin signatures | `nself_plugin_sdk` v0.0.x | `plugin-sdk-go` v0.1.x | v1.0.9 | Update `go.mod` in plugin source: `github.com/nself-org/plugin-sdk-go v0.1.0` | `[DEPRECATED] nself_plugin_sdk (since v1.0.9) → use github.com/nself-org/plugin-sdk-go` |
| API endpoints | (none removed in v1.0.9) | — | — | No action needed | — |
| Bundle name | `nMedia` bundle | `nTV` bundle | v1.0.9 | Update `nself plugin install` invocations: replace `nMedia` with `nTV`; update `git remote set-url origin https://github.com/nself-org/ntv` | `[DEPRECATED] nMedia bundle (since v1.0.9) → use nTV bundle` |
| Repo name | `nself-org/ntv` path was `ntv/` | same path (renamed in place) | v1.0.9 | Run: `git remote set-url origin https://github.com/nself-org/ntv` | — |
| Hasura column | `nself_sessions.user_agent` | use `device_type` + `device_name` | v1.0.9 | GraphQL queries: replace `user_agent` with `device_type` and `device_name`; EOL: 2027-04-17 | `@deprecated(reason: "use device_type and device_name instead , will be removed in v1.2.0")` |

---

## Detail

### Plugin SDK rename (plugin authors only)

If you author a custom plugin using the old `nself_plugin_sdk` Go package, update your `go.mod`:

```bash
# Remove old dependency
go get github.com/nself-org/nself_plugin_sdk@none

# Add new SDK
go get github.com/nself-org/plugin-sdk-go@v0.1.0
go mod vendor
```

Update imports in your plugin source:

```go
// Before (v1.0.8)
import "github.com/nself-org/nself_plugin_sdk"

// After (v1.0.9+)
import sdk "github.com/nself-org/plugin-sdk-go"
```

### Historical nMedia bundle migration

In v1.0.9, the `nMedia` bundle was renamed for the now-cancelled TV app.

At the time, the underlying plugins (media-processing, streaming, epg, tmdb,
podcast, recording, game-metadata, file-processing, subtitle-manager, vpn)
were unchanged.

### nself_sessions.user_agent deprecation (Hasura / GraphQL only)

This only affects consumers querying Hasura directly. If your code selects `user_agent`:

```graphql
# Before
query { nself_sessions { user_agent } }

# After (v1.0.9+)
query { nself_sessions { device_type device_name } }
```

The `user_agent` column returns data through 2027-04-17 (long-term support window) then is removed in v1.2.0.

---

## No-Change Audit (explicit)

The following were audited and confirmed unchanged in v1.0.8 → v1.0.9:

| Category | Count audited | Changes found |
|----------|--------------|---------------|
| Top-level CLI commands | 47 | 0 |
| CLI persistent flags | 12 | 0 |
| Core env vars (F09) | 200+ | 0 |
| ping_api endpoints | 14 | 0 (additive: `Sunset` header added) |
| Marketplace Worker endpoints | 14 | 0 |
| Webhook event payloads | 8 types | 0 |

---

## Related Pages

- [[Upgrade-From-v0.9]], full upgrade path from v0.9
- [[Migration-Roadmap]], three-version migration roadmap including upcoming v1.1.0 signals
- [[Changelog]], full v1.0.9 release notes

[[Home]]
