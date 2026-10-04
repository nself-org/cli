# Image Lock

← [[Config-Env-Vars]] · [[Compat-V15]]

---

Every container image the CLI generates or runs comes from one file: `internal/compose/images.yaml` (authored), compiled into `internal/compose/images.lock.json` (generated, embedded in the binary). No other Go file may spell an image reference; a scan test (`internal/compose/images_literal_scan_test.go`) fails the build if one appears.

## The lock

Each entry records `name`, `role` (`core`, `optional`, `monitoring`, `tool`, `plugin`), the fully qualified `repository`, `version`, the `index_digest` of the multi-arch index, the per-platform digests (`linux/amd64`, `linux/arm64`, `linux/arm/v7` when upstream publishes them), `legacy_ref`, `license`, `source` and `mirror`.

The first lock records what each effective reference resolved to on 2026-10-04. No version moved: `legacy_ref` is the exact string the CLI emitted before the lock existed.

## Pinning mode: `IMAGE_PINNING`

| Value | Image string | Default in |
|---|---|---|
| `legacy` | the entry's `legacy_ref`, byte-identical to earlier releases (`redis:7-alpine`, `nginx:alpine`) | v1.4.x |
| `lock` | `repository:version@sha256:<index digest>` | v1.5.0 (`NSELF_V15=1`) |

Set `IMAGE_PINNING=lock` in `.env` to opt a project in early. Moving to `lock` changes every image string, so the next `nself start` recreates the containers; plan for that on a serving box.

## Overrides

A `*_VERSION` key in the env cascade is an explicit override and is used as given, unpinned, unless it equals the lock version.

| Setting | `IMAGE_PINNING=lock` | `IMAGE_PINNING=legacy` |
|---|---|---|
| unset, or equal to the lock version (`REDIS_VERSION=7-alpine`) | `docker.io/library/redis:7-alpine@sha256:...` | `redis:7-alpine` |
| any other value (`REDIS_VERSION=6.2-alpine`) | `redis:6.2-alpine` (no digest) | `redis:6.2-alpine` |
| `POSTGRES_IMAGE` / `CS_N_IMAGE` | used as given | used as given |

A project digest file written by `nself update images` (`.nself-image-digests.json`) still applies to unpinned references; a lock reference already carries its digest and is left alone.

## How pins move

1. Edit the entry in `internal/compose/images.yaml` (a version bump is its own reviewed change).
2. `go run ./tools/imagelock -resolve` reads each index through the docker CLI (network), proves each digest addresses the index it came from, and rewrites the lock.
3. `go run ./tools/imagelock -check` verifies the lock offline: header, sorted, every entry has an index digest and `linux/amd64`, and the lock covers `images.yaml`. Adding an entry without resolving fails it.
4. `go run ./tools/imagelock -resolve -plugins images.json` also merges the plugins repository's `images.json` v1: `kind: plugin` entries become `plugin/<slug>`, `kind: upstream` entries keep their name, `kind: ci` entries are ignored.

An upstream that publishes no digest-addressable index, or misses a required platform, stops the run with the image named; raise it before locking.

## Mirror namespace

Each non-plugin entry names a mirror, `docker.io/nself/<name>`, at the same digest, for pulls when upstream deletes a tag. Plugin entries have no mirror: their repository is already ours.

## Probes

`tools/imagelist` prints `service<TAB>repository:version@digest` for every entry; `.github/workflows/default-images-pullable.yml` runs `docker manifest inspect` on each line and triggers on a change to `images.yaml` or `images.lock.json`.

## Accessors

`compose.LockedRef(name)` returns one entry, `compose.LockedImages()` all of them sorted by name, `compose.ImageRef(name, version)` the string for the active mode, and `compose.PinningMode()` the resolved mode.
