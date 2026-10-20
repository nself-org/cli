# Plugin security: release signatures and licence verification

How the CLI decides a plugin download can be trusted, and how it decides a licence response and a cached licence can be trusted. Release signatures come first; licence verification starts at [What ping_api signs](#what-ping_api-signs).

## Plugin release signatures

Every plugin release is signed by the pipeline that builds it (free plugins: the `release-signing` environment of `nself-org/plugins`; licensed plugins: the owner release machine). The registry and catalog entry carries `release_signature: {key_id, alg: "ed25519", sig}`. The CLI checks it before it extracts a single file.

### Trust root

The public keys are compiled into the CLI (`internal/plugin/verify/signature/keys.go`): a list of key ids with their tier, and a list of revoked ids. Nothing else adds a key: not the registry, not the plugin manifest, not an environment variable. The manifest `author_public_key` is still decoded so old manifests parse, but it is never used for trust.

- A key verifies only its own tier. A free-tier signature on a licensed entry (or the reverse) fails with E122.
- Key ids are derived: `plugins-` plus the first 16 hex characters of the SHA-256 of the 32-byte public key. Revoking an id revokes the key.
- The signed text is `nself-plugin-v1\n<slug>\n<version>\n<sha256 hex of the source tarball>\n`, raw Ed25519, signature as standard padded base64.
- A tier that has no key pinned yet cannot verify anything, so a signature on it is treated as missing (below). A tier with a pinned key never falls back. The free tier is pinned (table below); the licensed tier is pinned later.

#### Pinned production keys

| Tier | Key id | Public key file (sha256 of the file) | Pinned by |
|---|---|---|---|
| free | `plugins-2ebca8dfad98e19e` | `scripts/release/keys/plugins-release-free-2ebca8dfad98e19e.pub` in `nself-org/plugins` (`6720a63381e247173ed79f9cdf49e82df0e3f2ed4e7fa7a897b5bde6c6e84a4a`) | P7-PLUG-71 |
| licensed | not pinned yet | n/a | P7-PLUG-72 |

The compiled-in free key equals that file; `TestFreeTierPinned` fails when the table loses the key, holds a second free key, or lets one key serve both tiers, and `TestRealRelease` verifies a real signed `ci` 1.2.11 entry (and rejects the same entry with one flipped signature byte).

### Install order

1. Download to a temporary file. 2. SHA-256 against the registry checksum: a mismatch is E124. 3. Release signature against the pinned keys: E121 (bad), E122 (unknown key or wrong tier), E123 (revoked key). 4. Only then extract. A failure at 2 or 3 leaves the plugin directory untouched.

A binary package (a plugin that provides a command) is signed separately (contract v1.1). Each platform tarball is signed under its own domain, `nself-plugin-bin-v1`, over the message `nself-plugin-bin-v1\n<slug>\n<version>\n<platform>\n<sha256 hex of that platform tarball>\n`, with the same tier key. The registry entry carries `platform_signatures` next to `release_signature`, keyed by platform (`darwin-amd64`, `darwin-arm64`, `linux-amd64`, `linux-arm64`, `windows-amd64`). On a tier that has a pinned key the CLI installs a binary package only if the entry for the platform the CLI itself selected verifies over the sha256 the CLI computes from the downloaded bytes (never the registry's checksum field). A missing, unreadable, tampered, other-platform, other-version, other-slug or other-tier signature is refused (E121; E122 for a key the tier does not trust, E123 for a revoked key), in both modes, and the CLI never falls back to the source package for a platform package that fails. A source signature never verifies as a platform signature and the reverse, because the first lines differ. The licensed tier has no platform producer yet, so a pinned licensed tier keeps refusing platform packages. Accepted limits: `pluginType`, `binaryName` and `repository` are still unsigned registry fields, so a hostile registry can choose which signed tarball of a plugin and version installs (never unsigned bytes), and nothing gives freshness, so it can serve any earlier signed version to a fresh install (the signed-since floor and signed freshness own that). No tier is pinned yet, so nothing is refused today. The CLI does not verify GitHub build attestations.

Interim until the licensed key is pinned (P7-PLUG-72): `cron` and `notify` exist in both tiers, and the free floor now names them. The licensed tier has no trusted key yet, so even a signed licensed `cron` or `notify` counts as unsigned, and a slug named in any tier's floor needs a signature or a listed pre-signing release in the tier it claims: both are refused with E107. Every other licensed plugin keeps the old warn-only path. Do not cut a CLI release between the P7-PLUG-71 merge and P7-PLUG-72. The free pre-signing list is empty: an unsigned free release is refused whatever its version. The registry serves only signed releases of every free plugin.

Two limits to know: the tier of an entry comes from the registry, so while a tier has no pinned key (the state until the production keys ship) an entry can be moved to it and its signature is then read as missing; once both tiers are pinned a moved entry fails with E122. And the long-standing developer bypass (`NSELF_LICENSE_SKIP_VERIFY=1` together with `NSELF_LICENSE_SKIP_VERIFY_FORCE=1`, refused in production and staging, written to the audit log) can turn a missing signature into a skip, but it never skips a signature that is present on a pinned tier (a platform signature included, even when the registry strips `release_signature`), and it does not skip the checksum.

### Unsigned plugins

| Mode | A plugin with no verifiable `release_signature` |
|---|---|
| v1.4 (default) | installs with one warning |
| v1.5 (`NSELF_V15=1`) | refused with E107 |
| either mode, at or above the signed-since floor | refused with E107 |

`--allow-unsigned` is the opt-out for a local path or link install only; a registry install cannot use it. The policy is in place (`NSELF_PLUGIN_ALLOW_UNSIGNED`, honoured only when the caller marks the install local), but no local-path install command exists yet, so no command-line flag is registered; whichever command adds one wires it. A signature that is present, from a pinned tier, and invalid is always refused, in every mode.

### The signed-since floor

`internal/plugin/verify/signature/signed_since.json` is generated by `scripts/gen-signed-since.sh` from the release data of the first signed releases. For each tier it names the first signed version of each plugin. Once a tier's floor is filled, an entry at or above its floor, or for a plugin the floor does not name, must carry a signature in both modes; a registry that strips a signature from a newer release cannot turn the refusal into a warning. The same file carries a `presigning` allowlist of the real pre-signing releases (version and sha256, per tier and plugin). Once the floor or that list names a plugin, an unsigned release is accepted only if it is on the list for the tier the entry claims, so a forged older version label or a flipped tier is refused in both modes. An empty floor and an empty list apply no rule.

### ci gate binary (v1.4)

`nself ci` runs its gates through the `nself-ci` binary of the free `ci` plugin. The CLI finds it in this order and stops at the first hit:

1. The published plugin binary, `~/.nself/plugins/bin/nself-ci` (a regular file).
2. A dev build from an adjacent plugins checkout (`../plugins/free/ci`), as before.
3. The verified install seam: the CLI installs the signed `ci` plugin from the configured registry (`NSELF_PLUGIN_REGISTRY` or the default). It needs a release signature, a checksum and a platform signature for the CLI's own platform, all checked against the keys compiled into the CLI. A release that fails any of these is refused (E107, E121 to E124) and the error names `nself add ci`.

`PATH` is never searched, and a Go toolchain is never used to fetch the gate: the old `go install ...@latest` fallback was unpinned and unverified. `nself ci serve` resolves the gate the same way.

An earlier CLI may have left `~/.nself/bin/nself-ci` behind. If that is a regular file, the CLI renames it to `nself-ci.unverified-legacy` (never deleting it, and never overwriting an earlier copy) and prints one notice per run. A symlink, a directory, or a file in a read-only directory is left in place with the same notice.

`bash scripts/ci/probe-served-ci.sh --min-version <v>` checks the live registry before a release ships this behaviour: the free `ci` entry is signed, each of the four platform tarballs is covered by its platform signature and holds an executable `nself-ci`, and `nself ci` from the new binary matches the old one on a fixture, on the host and in a clean container.

### Rotation and revocation

To rotate, add the new key (new id) to `keys.go`, ship a CLI release, then start signing with it. To revoke, add the id to the revoked list and ship a release: a revoked key fails with E123 whatever the table holds, and it still counts as pinned so its tier cannot fall back to "unsigned".

### Error codes

| Code | Meaning |
|---|---|
| E121 | signature invalid or malformed |
| E122 | signed by an unknown key, or a key of the other tier |
| E123 | signed by a revoked key |
| E124 | downloaded tarball does not match the registry checksum |
| E107 | unsigned where a signature is required |

## Licence verification

## What ping_api signs

`POST /license/validate` returns a JSON body and signs those exact bytes into the
`X-NSelf-License-Sig` header (hex, Ed25519). It also issues a separate EdDSA licence JWT in
the body (`jwt`, `jwt_kid`, `jwt_expires_at`): `sub` is the SHA-256 of the licence key, plus
`tier`, `plugins`, `iat`, `exp` (24 hours), `iss: ping.nself.org`, `aud: nself-cli`. The
plugin list in the body is `plugins_allowed`. The same key signs the header and the JWT.

## What the CLI keeps

The cache (`~/.cache/nself/license.json`, mode 0600) stores the raw body, the header signature,
the JWT and its kid, plus copies of tier, plugins and expiry for callers. `fetched_at` is the
local clock when the response arrived; it is shown as cache age and is never signed or trusted
as a signature anchor.

## Verification (v1.5 mode, `NSELF_V15=1`)

- The header signature must verify over the stored body bytes with a key in `PingKeys`
  (`internal/license/keys.go`, committed public keys with their kids).
- The copies (tier, plugins, expiry) must equal the signed body, the body must say `valid: true`,
  and a stored JWT must carry `sub` equal to the cache's key hash.
- An entry with no signed body (every cache file written by an older CLI) is unverified.
- A present-but-invalid signature never verifies. An unverified cache is refused by the offline
  paths; it is never read as "licensed" and never as "free tier".
- `ParseLicenseJWT` accepts only alg `EdDSA` with a kid found in `PingKeys`, `iss ping.nself.org`,
  `aud nself-cli`. `none`, HS256, an unknown kid and a changed payload are rejected. It returns
  `exp`; the caller decides what an expired token means (the offline grace ladder owns that).

v1.4 mode keeps today's behaviour byte for byte: the legacy plugin field and the legacy cache
check. Capturing the new fields is on in both modes.

## Rotation

Add the new kid and public key to `PingKeys` and ship a release before ping signs with it;
drop the old kid after the grace window (see the key rotation runbook in the web repo).

## Keys, dev keys and failing closed

- Release and default builds (goreleaser, `go install`, `go test`) trust only the committed
  `PingKeys`. `LICENSE_PUBLIC_KEY_OVERRIDE` and any linker-injected key are ignored.
- A build tagged `nself_devkeys` (never used by goreleaser, homebrew or a release script) also
  trusts `LICENSE_PUBLIC_KEY_OVERRIDE` and `-X ...license.licensePubKeyHex`, and prints one
  warning per process the first time it does.
- There is no zero-key skip. An absent, nil, short or all-zero key is dropped; with no usable key
  nothing verifies, so a licence response without a valid `X-NSelf-License-Sig` is rejected and
  the CLI falls back to the cache, which also fails to verify. A failed check is never read as
  "licensed" and never as "free tier".
- `Validate` and `ValidateFull` both store the raw body and header signature, so the cache they
  write verifies in v1.5 mode.

## What a reply must name (v1.5)

From v1.5 a reply from ping counts only if bytes ping signed name the requesting licence and a window:

- `POST /license/validate` (main reply): the signed body carries `jwt`; its `sub` must be sha256 of the key asked about, and the reply is trusted from `iat` (5 minutes of clock skew allowed) until `exp` or `iat` + 24 h, whichever comes first, so a jwt with a far `exp` cannot keep a replayed reply fresh. A reply for another key, or without a jwt, is refused.
- `POST /license/validate?bundle=<name>`: the signed body must carry `bundle` (equal to the bundle asked about), `key_hash` (sha256 of the key), and `issued_at`/`expires_at` (unix seconds, at most 24 h apart, containing now), with an `X-NSelf-License-Sig` header over the exact body. Until ping signs this, v1.5 refuses every bundle reply.
- Cache age is the older of the local `fetched_at` and the signed jwt `iat`, so re-stamping `fetched_at` cannot extend the offline grace window.

Offline decisions from the cache (v1.5):

- `NSELF_LICENSE_FAIL_OPEN=1` has no cache-age limit but still stops at the signed licence term: expiry plus the 30-day post-expiry grace.
- The post-expiry grace is read-only once the cache is older than the 7-day offline ceiling.
- A clock behind the cache's signed `iat`, or more than 10 minutes behind the highest time earlier checks trusted (`license.clock` next to the cache), makes the cache untrusted until a verified online reply resets it.

v1.4 behaviour is unchanged except that a present-but-invalid signature is refused in every mode.

## Checking the code

`bash scripts/mutation.sh internal/license` runs go-gremlins (pinned release, fetched from the
module proxy into a temp dir) over the whole package and exits 1 if any mutant survives. This
full-package run is the licence mutation gate: `.github/workflows/license-mutation.yml` runs it
nightly, on demand, and on every pull request that touches `internal/license/` or the script.

Add `--only a.go,b.go` to judge just those files of the package: every other file is excluded from
the run, and any surviving or uncovered mutant in a listed file exits 1. Use it for a quick check of
the licence verdict files while working (`validate.go,validator.go,validator_validate.go,checker.go,cache.go,cache_entry.go,keys.go,keys_release.go,keys_devkeys.go,jwt.go,trust.go`);
it does not replace the full-package gate. Files tagged `nself_devkeys` run in a second pass with
that tag. A mutant `--only` can only report as not covered (a const initialiser has no coverage
block) is listed with a reason in `MUTATION_EQUIVALENT` in the script.
