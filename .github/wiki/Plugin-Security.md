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

## Not in this version

The dev-key confinement (override and ldflags key only in `-tags nself_devkeys` builds) and the
removal of the zero-key skip are tracked on P7-PLUG-63 and are not part of this change.
