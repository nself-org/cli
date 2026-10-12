# Image Lock

← [[Config-Env-Vars]] · [[Compat-V15]]

---

Every container image the generated compose files and the CLI's own tools (ACME client, admin pull, monitoring fragments) use comes from one file: `internal/compose/images.yaml` (authored), compiled into `internal/compose/images.lock.json` (generated, embedded in the binary). A scan test (`internal/compose/images_literal_scan_test.go`) fails the build when a non-test Go file under `internal/` or `cmd/` spells an image reference as a single string literal.

What the scan does not see: literals assembled from parts (for example `"postgres:" + version`, tracked as debt D-0213), Dockerfile templates inside raw strings (scaffold output), `tools/`, untagged repository strings, and the dead `docker-compose.monitoring.yml`. Three allowlisted tool images remain (plugin author tool, CI job image, controlplane test harness).

## The lock

Each entry records `name`, `role` (`core`, `optional`, `monitoring`, `tool`, `plugin`), the fully qualified `repository`, `version`, the `index_digest` of the multi-arch index, the per-platform digests (`linux/amd64`, `linux/arm64`, `linux/arm/v7` when upstream publishes them), `legacy_ref`, `license`, `source` and `mirror`.

The first lock records what each effective reference resolved to on 2026-10-04. No version moved: `legacy_ref` is the exact string the CLI emitted before the lock existed. `legacy_ref` stays frozen: a pin bump moves `version` (and the digests) only, so legacy mode keeps emitting the pre-lock string. The lock test checks that `legacy_ref` names the same repository with a tag; it does not require `legacy_ref` to equal `repository:version`.

## Pinning mode: `IMAGE_PINNING`

| Value | Image string | Default in |
|---|---|---|
| `legacy` | the entry's `legacy_ref`, byte-identical to earlier releases (`redis:7-alpine`, `nginx:alpine`) | v1.4.x |
| `lock` | `repository:version@sha256:<index digest>` | v1.5.0 (`NSELF_V15=1`) |

Set `IMAGE_PINNING=lock` in `.env` to opt a project in early. Moving to `lock` changes every image string, so the next `nself start` recreates the containers; plan for that on a serving box.

## Overrides

A `*_VERSION` key in the env cascade is an explicit override and is used as given, unpinned, unless it equals the lock version or the `legacy_ref` tag. The `legacy_ref` tag is what the config defaults and the `.env` files written by `nself setup` carry (`HASURA_VERSION=v2.44.0`), so it counts as "the default" and stays pinned after a bump. To run the pre-lock version itself, use `IMAGE_PINNING=legacy`.

| Setting | `IMAGE_PINNING=lock` | `IMAGE_PINNING=legacy` |
|---|---|---|
| unset, or equal to the lock version or the `legacy_ref` tag (`REDIS_VERSION=7-alpine`) | `docker.io/library/redis:7-alpine@sha256:...` | `redis:7-alpine` |
| any other value (`REDIS_VERSION=6.2-alpine`) | `redis:6.2-alpine` (no digest) | `redis:6.2-alpine` |
| `POSTGRES_IMAGE` / `CS_N_IMAGE` | used as given | used as given |

An `IMAGE_PINNING` value other than `lock` or `legacy` (case ignored) is an error: `nself build` and every command that loads the config refuse to run rather than fall back silently.

`nself update images` writes a project digest file (`.nself-image-digests.json`); `nself build` does not read it today (debt D-0673). A lock reference already carries its digest.

## Valkey, the default cache

Redis 7.4 and later is RSALv2/SSPLv1, so the lock carries a `valkey` entry (`docker.io/valkey/valkey:8.1-alpine`, BSD-3-Clause) and the `redis` compose service runs it by default in v1.5. The service name, `REDIS_*` variables and `REDIS_URL` stay. The engine is chosen by `REDIS_ENGINE` (`valkey` or `redis`), then by the engine of the project lock's `redis` pin, then by the mode default (v1.4 `redis`, v1.5 `valkey`):

| Project | Result |
|---|---|
| v1.5, new project (no lock, no existing compose) | Valkey from the lock entry, volume `valkey_data` |
| v1.5, lock pins a redis image | stays Redis; `nself build --plan` shows no cache change |
| v1.5, no lock, existing `docker-compose.yml` runs redis | stays Redis (the first lock takes the pin from that file) |
| `REDIS_ENGINE=valkey`, any mode | Valkey; a v1.4 project switches with `nself config set REDIS_ENGINE valkey`, then `nself build --plan`, `nself build`, `nself start` |
| `REDIS_ENGINE=redis`, any mode | Redis, whatever the lock pins |

Valkey runs `valkey-server` with the same flags, user, port and network, and its health check is `valkey-cli ping` (no reliance on `redis-*` symlinks). A `REDIS_VERSION` equal to the Redis default (`7-alpine`) means the Valkey default; any other value is used as the Valkey tag.

Queued jobs are not migrated: Valkey starts on an empty `valkey_data` volume, and the plan shows a stateful recreate of the `redis` service. `redis_data` is kept, so setting `REDIS_ENGINE=redis` again reuses it.

## Auth: nhost/auth

Upstream renamed `nhost/hasura-auth` to `nhost/auth`; the old name stops at 0.42.x. `nhost/auth:0.36.0` has the same index digest as `nhost/hasura-auth:0.36.0` (`sha256:2e0713ef…`), so every mode now names `nhost/auth` and nothing that runs changes. The image string does change, so the first `nself start` after the upgrade pulls `nhost/auth:0.36.0` and recreates the auth container once. The lock carries two auth entries: `auth-v14` (`docker.io/nhost/auth:0.36.0`) and `auth` (`docker.io/nhost/auth:0.52.0`, the current release).

| Project | Auth image |
|---|---|
| v1.4 | `auth-v14`: `docker.io/nhost/auth:0.36.0@sha256:...` in lock mode, `nhost/auth:0.36.0` in legacy mode |
| v1.5, no `AUTH_ENCRYPTION_KEY` | `auth-v14`; `nself build` prints one notice |
| v1.5, `AUTH_ENCRYPTION_KEY` set (64 hex characters) before the first build, so no auth pin yet | `auth` (0.52.0) |
| the lock pins 0.43.0 or later (0.52.0) but the key is missing or malformed | `nself build` stops with an error naming `AUTH_ENCRYPTION_KEY` |
| `AUTH_VERSION=0.52.0`, no lock pin | `auth` with the key; without it `auth-v14` and the notice |
| the project lock pins auth 0.36.0 (either repository name) | the pin, in both modes, until `nself update` moves it |
| `AUTH_VERSION` set to a release from 0.43.0 (`0.45.0`) | `nhost/auth:<tag>`, unpinned, in any mode; without the key `nself build` prints a notice |
| `AUTH_VERSION=latest`, a release before 0.43.0 (`0.42.0`) or another tag | `nhost/hasura-auth:<tag>`, the exact string earlier releases rendered (the frozen line) |

`AUTH_VERSION=0.36.0` (what `nself init` writes into `.env`) and `AUTH_VERSION=0.52.0` are defaults, not overrides: neither drops a lock pin.

nhost/auth 0.43.0 and later refuse to start without `AUTH_ENCRYPTION_KEY` (32 bytes, hex) and encrypt every TOTP secret in `auth.users` with it on first start. The CLI does not generate the key yet, so v1.5 moves to the current release only once you set it. Setting it moves a project that has no auth lock pin (no `nself.lock.yaml` yet, or no auth line in it) on its next build. Keep the key with your backups: after the first start, losing or changing it locks out every user who signs in with TOTP.

### Upgrade an existing project (one way)

1. Take a verified backup: `nself backup create`, then `nself backup verify <id> --restore-test`. Copy `nself.lock.yaml` aside too: it pins auth 0.36.0.
2. Add the key to `.env.secrets`: `AUTH_ENCRYPTION_KEY=<output of openssl rand -hex 32>`.
3. Move to the new release. A v1.4 project without `nself.lock.yaml` sets `AUTH_VERSION=0.52.0`. Every other project has an auth pin: a v1.5 build pins the image the project already runs (its first lock takes it from the existing `docker-compose.yml`), and the pin wins over `AUTH_VERSION=0.52.0`. A pinned project moves with `nself update`: in v1.5 with the key, `nself update --plan` shows the auth move with a one-way note and `nself update --yes` applies it. Without the key (or in v1.4) update keeps 0.36.0, and it never moves a pin that already migrated the database back.
4. `nself build`, then `nself start`. Auth migrates its schema and encrypts the TOTP secrets on that first start.

Signed-in users stay signed in: a refresh token issued by 0.36.0 is accepted once by 0.52.0, which returns a new refresh token and rejects the old one from then on (refresh-token rotation, introduced upstream in 0.38.0). Clients must store the new token; the Nhost SDKs do.

### Roll back

Auth migrates its schema one way, so rolling back means restoring the backup from step 1:

1. `nself stop auth hasura`, so neither reconnects during the restore.
2. `nself db reset --yes` recreates the database. A plain restore over the migrated database fails (E207: `pg_restore --clean` cannot drop `auth.users` while objects the newer release added depend on it; debt D-0663).
3. `nself backup restore <id> --only pg --yes`.
4. Put the saved `nself.lock.yaml` back. A project without a lock removes `AUTH_VERSION=0.52.0` and `AUTH_ENCRYPTION_KEY` instead (keep a copy of the key): while the key is set, v1.5 renders 0.52.0 again and re-migrates the restored database.
5. `nself build`, then `nself start`.

Pinning back without the restore is unsupported: 0.36.0 starts on the migrated schema, but every TOTP sign-in fails because the secrets are now encrypted. `scripts/ci/auth-smoke.sh` proves the upgrade and both rollback paths on Linux with users, sessions, a refresh token and a TOTP user (`NSELF_V15=1 bash scripts/ci/auth-smoke.sh`; it needs Docker, Go, curl, python3 and free ports 80, 443, 4000, 5432 and 8080).

## How pins move

1. Edit the entry's `version` in `internal/compose/images.yaml` (a version bump is its own reviewed change). Leave `legacy_ref` alone. The weekly bump (`tools/imagebump`, `.github/workflows/image-pin-bump.yml`) does the same.
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
