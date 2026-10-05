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
//   - Trust file (ParseKeysFile): UTF-8 text, one entry per line,
//     "<key-id><blanks><base64 public key>"; blanks are spaces or tabs; a
//     trailing CR is dropped; blank lines and lines whose first non-blank
//     character is # are ignored; a # after content is NOT a comment and makes
//     the line malformed (three fields). A line over 4096 bytes, a file over 1
//     MiB, more than 1024 entries, a duplicate id, a bad id, bad base64 or a
//     wrong key length is an error and no keys are returned.
//   - Revoked file (ParseRevokedFile): one key id per line, same comment rules;
//     duplicates are collapsed, first-seen order is kept.
//   - DSSE v1: PAE is "DSSEv1 <len(type)> <type> <len(payload)> <payload>"
//     with decimal lengths without leading zeros. Envelope.Payload and each
//     signature are base64 standard with padding (strict as above). An envelope
//     needs a non-empty payloadType and 1 to 16 signatures with distinct key
//     ids; it is accepted when at least one signature verifies under the
//     verifier, extra signatures that fail are ignored, and when none verifies
//     the first signature's error is returned. VerifyEnvelope returns the
//     payload only on success.
//   - Key validity: Key.NotBefore is inclusive and Key.NotAfter exclusive; a
//     zero time means unbounded. Trust files carry no validity, so keys from
//     them never expire; revocation is the lever for those.
//   - Key lookup: the KeyLookup is called once per Verify with the signature's
//     key id; its error is returned wrapped (never treated as success) and a
//     returned key whose ID differs from the one asked for is ErrUnknownKey.
//
// # Failure policy
//
// Verification fails closed: every malformed input, unknown or revoked key,
// purpose, scope or validity mismatch and bad signature returns an error
// matching a sentinel with errors.Is, and nothing in this package panics on
// hostile input (fuzzed). Error text names the key id (quoted) and never key
// material. Public keys are not secrets, so comparisons use plain or
// constant-time equality without a timing concern; ed25519.Verify is the only
// signature comparison and is the standard library's.
//
// Never place a production key in this package, its tests or its fixtures.
package signing
