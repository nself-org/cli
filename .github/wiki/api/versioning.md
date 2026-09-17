# API Versioning, Plugin Stability Framework (G2–G11)

> This page covers the plugin API stability framework shipped at v1.0.9. Each plugin exposes
> a versioned HTTP API governed by the policies described here.

## Overview

Every ɳSelf plugin uses SemVer for its HTTP API surface. The version lives in two canonical places:

1. `plugin.json`, the `"api_version"` field in the plugin manifest.
2. `cli/internal/deprecation/registry.yaml`, the ecosystem-wide plugin registry used by the CLI.

The public deprecation registry is also available as a JSON endpoint at:

```
GET https://ping.nself.org/api/deprecation-registry.json
```

## Gates G2–G11

| Gate | What It Does |
|------|--------------|
| **G2** | Each plugin.json has an `api_version` SemVer field |
| **G3** | Each plugin has a `CHANGELOG.md` in its source directory |
| **G4** | `calendar.go` computes sunset dates from SemVer removal versions |
| **G5** | Plugin HTTP handlers inject `Deprecation: true` and `Sunset: <date>` headers on deprecated paths |
| **G6** | `nself api deprecation-check --plugin <name>` checks a single plugin |
| **G7** | `nself start` runs a client-side compat check; warns if installed plugin has deprecated endpoints |
| **G8** | `GET /api/deprecation-registry.json` on ping.nself.org publishes the full registry |
| **G9** | `nself api changelog <plugin>` shows the deprecation calendar for a plugin |
| **G10** | Admin UI panel at `/api/versioning` shows all plugins with API status |
| **G11** | `bundles/.github/workflows/api-compat.yml` gates PRs that add endpoints without a grace period |

## Versioning Policy

| Change type | Version bump |
|-------------|--------------|
| New endpoint added | Patch (`1.0.0` → `1.0.1`) |
| Endpoint behavior changed, backward-compatible | Minor (`1.0.0` → `1.1.0`) |
| Endpoint deprecated (grace period added) | Minor |
| Endpoint removed after grace period | Major (`1.x.x` → `2.0.0`) |

## Deprecation Lifecycle

1. The endpoint is marked `deprecated_in: <version>` in `registry.yaml`.
2. Requests to that path receive `Deprecation: true` and `Sunset: <RFC-1123-date>` response headers.
3. After the `removed_in` version ships, the path returns `410 Gone`.

## Client Migration

When `nself start` prints a deprecation warning:

```
[API WARNING] plugin 'ai' v1.1.0: endpoint '/v1/complete' was deprecated in v1.2.0,
removed in v2.0.0. Use '/v2/complete' instead.
```

Update your integration to use the replacement path listed. Check the current calendar with:

```bash
nself api changelog ai
```

## CI Gate (G11)

Pull requests to `plugins-pro` that add a new endpoint to `plugin.json` without a `deprecated_in`
grace period on any replaced endpoint are blocked by the `api-compat.yml` workflow.

To override: a reviewer must add the `breaking-change-approved` label. This is audited in release
notes at the next SemVer bump.

## Registry JSON Endpoint (G8)

```bash
curl https://ping.nself.org/api/deprecation-registry.json | jq .
```

Response shape:

```json
{
  "schema_version": "1",
  "generated_at": "2026-04-23T00:00:00Z",
  "lts_baseline": "1.0.9",
  "lts_window_end": "2027-04-17",
  "plugins": [
    {
      "name": "ai",
      "api_version": "1.3.0",
      "deprecated_endpoints": []
    }
  ]
}
```

## See Also

- [[cmd-api]], `nself api` CLI reference
- [[cmd-api#nself-api-deprecation-check]], deprecation-check flags
- [[cmd-api#nself-api-changelog]], changelog subcommand
- [Admin UI API Versioning Panel](http://localhost:3021/api/versioning), local Admin GUI
