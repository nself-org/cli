# signing

Ed25519 signature verification for the nSelf CLI, CI plugin and agent. Standard
library only, additive API. The exact encoding rules live in the package doc
(`doc.go`); each has a test.

## Purposes

`plugins`, `agent`, `ci-release`, `ci-node`, `ci-audit`. A `Verifier` is pinned
to one purpose (and optionally one scope), so a `ci-release` signature never
verifies as a plugin or agent signature (`ErrWrongPurpose`).

## API

| Name | Use |
| --- | --- |
| `Key{ID, Purpose, Scope, Public, NotBefore, NotAfter}` | a public key and its trust attributes |
| `KeyID(p, pub)` | `<purpose>-<first 16 hex of sha256(pub)>` |
| `NewVerifier(p, keys, revoked, opts...)` | fixed key set |
| `NewLookupVerifier(p, lookup, opts...)` | keys resolved per call (`KeyLookup`) |
| `WithScope`, `WithRevoked`, `WithClock` | verifier options |
| `Verify(msg, Signature)`, `VerifyContext` | returns nil or a sentinel error |
| `ErrUnknownKey`, `ErrRevoked`, `ErrWrongPurpose`, `ErrWrongScope`, `ErrBadSignature`, `ErrMalformed`, `ErrExpired`, `ErrNotYetValid` | match with `errors.Is` |
| `Signer`, `NewEd25519Signer`, `ParsePKCS8PEM` | signing side |
| `EncodeSig`, `DecodeSig` | strict base64 standard signature text |
| `ParseKeysFile`, `ParseRevokedFile` | `.nself/trust/<purpose>.keys` and `.revoked` |
| `PAE`, `Envelope`, `SignEnvelope`, `VerifyEnvelope` | DSSE v1 |
| `signingtest.NewKey(t, p)` | fresh in-memory key for tests |

Signatures are raw Ed25519, base64 standard, identical to
`openssl pkeyutl -sign -rawin ... | base64 | tr -d '\n'`.

## Trust file

```
# <key-id> <base64 raw 32-byte public key>
ci-release-0123456789abcdef  AAAA...=
```

`.revoked` is one key id per line. Whole-line `#` comments only.

## Fail closed

Every malformed input, unknown or revoked key, purpose, scope or validity
mismatch and bad signature returns an error; `VerifyEnvelope` returns no
payload on failure. The package is fuzzed and mutation-tested
(`bash scripts/mutation.sh sdk/go/signing`). No production key belongs in this
package; test keys are generated in-test or live in `testdata/`.
