// Package signing is the one Ed25519 signature implementation for the nSelf
// CLI, the CI plugin and the agent (ADR 0024 section 8). It is standard library
// only and its API is additive.
//
// # Purposes and scope
//
// A key belongs to exactly one Purpose (plugins, agent, ci-release, ci-node,
// ci-audit). A Verifier is pinned to one purpose and optionally one scope; a
// signature made by a key of another purpose never verifies (ErrWrongPurpose),
// so a ci-release signature is never a plugin or agent signature.
//
// # Canonical encoding decisions
//
// These are fixed here and each has a test:
//
//   - Signature: the raw 64-byte Ed25519 signature, base64 standard alphabet
//     with padding, no line breaks, no whitespace, no trailing bits. DecodeSig
//     is strict: any newline, space, URL-safe character, missing padding or
//     non-zero trailing bit is ErrMalformed. This is the output of
//     `openssl pkeyutl -sign -rawin` piped through `base64 | tr -d '\n'`.
//   - Public key: the raw 32 bytes, base64 standard with padding (the form in
//     trust files). A length other than 32 is ErrMalformed.
//   - Key id: 1 to 128 characters of [A-Za-z0-9._-], compared byte-for-byte
//     (case sensitive, no normalisation). KeyID derives
//     "<purpose>-<first 16 lowercase hex of sha256(pub)>".
//   - Messages are signed as given, byte for byte. Domain separation is the
//     caller's: put a purpose label in the message (DSSE does it with PAE).
//   - Key ids are always derived. Every key held by a verifier or returned by a
//     lookup, and every line of a trust file, has id == KeyID(purpose, public
//     key), compared exactly (so upper-case hex is refused). A key cannot be
//     listed under a chosen id, nor under another purpose's id. Because the id
//     is a function of the key, revoking the id revokes the key.
//   - One public key appears once per key set (any purpose); a second entry for
//     the same 32 bytes is ErrMalformed, so no alias id can outlive a
//     revocation.
//   - Small-order keys are refused as ErrMalformed at every entry point
//     (NewVerifier, ParseKeysFile and a lookup's result). Go's ed25519.Verify
//     accepts the identity point as a public key, under which one fixed
//     signature (R = identity, S = 0) verifies every message. The refusal is
//     the libsodium blocklist (ge25519_has_small_order, ref10): the encodings
//     of y = 0, y = 1, y = p-1, the two order-8 points, and the non-canonical
//     y = p and y = p+1, compared with the sign bit ignored. Keys with a mixed
//     torsion component are not detected; they cannot be forged by third
//     parties.
//   - Trust file (ParseKeysFile): UTF-8 text, one entry per line,
//     "<key-id><blanks><base64 public key>"; blanks are spaces or tabs; a
//     trailing CR is dropped; blank lines and lines whose first non-blank
//     character is # are ignored; a # after content is NOT a comment and makes
//     the line malformed (three fields). A line over 4096 bytes, a file over 1
//     MiB, more than 1024 entries, a duplicate id or public key, an id that is
//     not derived, bad base64, a wrong key length or a small-order key is an
//     error and no keys are returned.
//   - Revoked file (ParseRevokedFile) and the revoked list of NewVerifier: each
//     entry must have the shape of a derived id, "<purpose>-<16 lowercase hex>"
//     with one of the five purposes; anything else (upper-case hex, a custom
//     id, a wrong length) could never match a key and is ErrMalformed rather
//     than a silent no-op. The file has one id per line, same comment rules as
//     the trust file; duplicates are collapsed, first-seen order is kept.
//   - DSSE v1: PAE is "DSSEv1 <len(type)> <type> <len(payload)> <payload>"
//     with decimal lengths without leading zeros. Envelope.Payload and each
//     signature are base64 standard with padding (strict as above). An envelope
//     needs a non-empty payloadType and 1 to 16 signatures with distinct key
//     ids. VerifyEnvelope succeeds only when at least one signature verifies
//     under a trusted key and no signature that names a known key id fails;
//     signatures by unknown key ids are ignored (rotation). A malformed
//     signature, or one by a known but revoked, expired, wrong-purpose,
//     wrong-scope or non-verifying key, fails the envelope. It returns the
//     verified key ids in envelope order. On failure it returns nothing but the
//     most specific error (revoked, expired, not yet valid, wrong purpose,
//     wrong scope, bad signature, malformed, other; ties go to the earlier
//     signature), or ErrUnknownKey when every signature named an unknown key.
//     Consumers that need exactly one signer (the release manifest) check
//     len(keyIDs) themselves.
//   - Key validity: Key.NotBefore is inclusive and Key.NotAfter exclusive; a
//     zero time means unbounded. Trust files carry no validity, so keys from
//     them never expire; revocation is the lever for those.
//   - Key lookup: the KeyLookup is called once per Verify with the signature's
//     key id; its error is returned wrapped (never treated as success) and a
//     returned key whose ID differs from the one asked for is ErrUnknownKey. A
//     returned key must pass the same shape checks as a fixed key.
//   - Nil safety: every Verifier method on a nil or zero Verifier fails closed
//     (Verify returns ErrMalformed; Purpose returns "").
//
// # Failure policy
//
// Verification fails closed: every malformed input, unknown or revoked key,
// purpose, scope or validity mismatch and bad signature returns an error
// matching a sentinel with errors.Is (a reader failure in a file parser or a
// lookup's own error is wrapped and matches no sentinel), and nothing in this package panics on
// hostile input (fuzzed). Error text names the key id (quoted) and never key
// material. Public keys are not secrets, so comparisons use plain or
// constant-time equality without a timing concern; ed25519.Verify is the only
// signature comparison and is the standard library's.
//
// Never place a production key in this package, its tests or its fixtures.
package signing
