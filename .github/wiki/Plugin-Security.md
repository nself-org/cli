# Plugin security: licence verification

How the CLI decides a licence response and a cached licence can be trusted.

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

- `POST /license/validate` (main reply): the signed body carries `jwt`; its `sub` must be sha256 of the key asked about and `iat`/`exp` must contain now. A reply for another key, or without a jwt, is refused.
- `POST /license/validate?bundle=<name>`: the signed body must carry `bundle` (equal to the bundle asked about), `key_hash` (sha256 of the key), and `issued_at`/`expires_at` (unix seconds, at most 24 h apart, containing now), with an `X-NSelf-License-Sig` header over the exact body. Until ping signs this, v1.5 refuses every bundle reply.
- Cache age is the older of the local `fetched_at` and the signed jwt `iat`, so re-stamping `fetched_at` cannot extend the offline grace window.

v1.4 behaviour is unchanged except that a present-but-invalid signature is refused in every mode.

## Checking the code

`bash scripts/mutation.sh internal/license` runs go-gremlins (pinned release, fetched from the
module proxy into a temp dir) and exits 1 if any mutant survives.

Add `--only a.go,b.go` to judge just those files of the package: every other file is excluded from
the run, and any surviving or uncovered mutant in a listed file exits 1. The licence verdict files
are checked this way (`validate.go,validator.go,validator_validate.go,checker.go,cache.go,cache_entry.go,keys.go,keys_release.go,keys_devkeys.go,jwt.go`).
Files tagged `nself_devkeys` run in a second pass with that tag. The few equivalent mutants (no test
can tell them from the original) are listed with a reason in `MUTATION_EQUIVALENT` in the script.
