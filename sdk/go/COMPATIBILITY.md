# plugin-sdk-go Compatibility Matrix

The shared Go SDK for nSelf plugins. Every licensed plugin (ai, mux, claw,
voice, browser, notify, cron, chat, livekit, etc.) and every free plugin
depends on this module.

## Import path

```go
import "github.com/nself-org/cli/sdk/go/v2/plugin"
```

The module path is `github.com/nself-org/cli/sdk/go/v2` (see [`go.mod`](go.mod)).
The module lives in the `sdk/go/` directory of the `nself-org/cli` repository.

## Version mapping

The module path ends in `/v2`, so Go resolves only tags named
`sdk/go/v2.M.P`: directory prefix `sdk/go/`, semantic version `v2.M.P`.
CLI release `v1.M.P` publishes SDK `v2.M.P`.

| nSelf CLI release | Git tag | `go get` version |
| --- | --- | --- |
| `v1.5.0` | `sdk/go/v2.5.0` | `github.com/nself-org/cli/sdk/go/v2@v2.5.0` |
| `v1.4.13` | `sdk/go/v2.4.13` | `github.com/nself-org/cli/sdk/go/v2@v2.4.13` |

[`scripts/sdk-tag.sh`](scripts/sdk-tag.sh) owns this mapping. It prints the tag
for a CLI version and exits 1 for any major other than 1. The publish workflow
(`sdk-publish-go.yml`) and the nightly coherence check (`sdk-coherence-check.yml`)
both call it. The [`doc.go`](doc.go) `Version` constant stays the CLI version.

[`scripts/resolve-test.sh`](scripts/resolve-test.sh) proves the mapping without
network: it resolves `sdk/go/v2.99.0` from a local clone and checks that the
old tag form does not resolve.

Pre-release CLI tags (`v1.5.0-rc.1`) publish no SDK tag: SDK tags are immutable,
so `sdk-tag.sh` refuses a pre-release and the publish workflow skips it. The
workflow only tags from a release tag ref (`v1.M.P` or `cli-sdk-go/v1.M.P`), never
from a branch.

### The `license` package was renamed

The module zip includes the repository-root `LICENSE` file, and a package
directory named `license/` collides with it case-insensitively, so the go command
could not build any version of the module. The package is now `licensing`
(`github.com/nself-org/cli/sdk/go/v2/licensing`); `modzip_test.go` guards against
a repeat.

### Why the old tags do not resolve

Every tag published before this mapping is unusable, and none is ever deleted
or moved. The proxy ignores them once a valid `sdk/go/v2.M.P` tag exists.

| Old form | Why Go rejects it |
| --- | --- |
| `sdk/go/v1.1.3` to `sdk/go/v1.3.5`, `sdk/go/main` | `go.mod` at that revision declares the `/v2` module path, but a `v1.x.y` tag cannot carry a `/v2` module (post-v1 module path at a v1 version) |
| `sdk/go/v2/v1.3.6` to `sdk/go/v2/v1.4.12` | The directory prefix `sdk/go/v2/` points at a `v2/` subdirectory that does not exist, and the version is still `v1.x.y` |

## Current versions

| Component | Version | Notes |
| --- | --- | --- |
| `plugin-sdk-go` | CLI version | See [`doc.go`](doc.go) `Version` constant; module version is `v2.M.P` for CLI `v1.M.P` |
| nSelf CLI (min) | **1.0.9** | Plugins declaring `minNselfVersion` below this are rejected |
| Go toolchain | **1.25.0+** | `go.mod` declares `go 1.25.0` |

## Compatibility guarantees

`plugin-sdk-go` follows strict SemVer.

| Change class | Major | Minor | Patch |
| --- | --- | --- | --- |
| Add new package (e.g. `tracing/`) | | x | |
| Add new exported function or type | | x | |
| Add new optional field on an `Options` struct | | x | |
| Fix bug without changing signatures | | | x |
| Rename / remove any exported symbol | x | | |
| Change behavior of existing function in a way that breaks callers | x | | |
| Bump min Go version | x | | |

Plugins specify their minimum SDK version via `sdk.CheckMinSDK(required)` at
startup. Plugin manifests (`plugin.json`) also declare `minSdkVersion` for
offline verification by the CLI.

## CLI ↔ plugin compatibility

Plugins declare their CLI range via `plugin.json`:

```json
{
  "minNselfVersion": "1.0.9",
  "maxNselfVersion": ""
}
```

Empty `maxNselfVersion` means "no upper bound". CLI refuses to install a
plugin whose declared range does not include the running CLI. Use
`sdk.CheckCLICompat(currentCLI, minCLI, maxCLI)` inside a plugin to enforce
the same check at runtime.

## Supported Go versions

| Go | plugin-sdk-go support |
| --- | --- |
| 1.24 and older | not supported (`go.mod` requires 1.25.0) |
| 1.25 | **required minimum** |
| 1.26+ | supported (best-effort) |

## Supported CLI versions

plugin-sdk-go 0.1.x targets CLI **1.0.9 and newer**. CLI 1.0.5-1.0.8 predate
the Go plugin runtime contract and are not supported.

## Release policy

- Patch releases every ~2 weeks or on critical fix.
- Minor releases bundled with CLI LTS ticks.
- Major releases only when breaking changes cannot be absorbed by a minor.
- Every release ships a migration note in [`CHANGELOG.md`](CHANGELOG.md) when
  downstream plugins must update.

## Runtime version checks

Every plugin's `main.go` should include:

```go
if err := sdk.CheckMinSDK("0.1.0"); err != nil {
    log.Fatalf("incompatible plugin-sdk-go: %v", err)
}
if err := sdk.CheckCLICompat(os.Getenv("NSELF_CLI_VERSION"), "1.0.9", ""); err != nil {
    log.Fatalf("incompatible nSelf CLI: %v", err)
}
```

## Deprecation policy

Deprecated symbols stay for a minimum of **two minor releases** before removal.
They are marked with a `// Deprecated: ...` comment and surface a `go vet`
warning through the standard lint path.
