// Package binsig checks the release signature a `nself update upgrade
// --binary-url` install must carry (P7-WIRE-22).
//
// Purpose: refuse fetched CLI code whose digest is not signed by a trusted key.
// Inputs: the lowercase hex SHA-256 digest and a hex Ed25519 signature over
// that digest text.
// Outputs: nil when a trusted key signed the digest, else a descriptive error.
// Constraints: runs before any backup or download; fails closed. Trusted keys
// are the committed Keys (none yet) plus the operator-pinned key in KeyEnv.
package binsig

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// KeyEnv names the operator-pinned release public key (hex Ed25519). It is
// read from the process environment, never from a request.
const KeyEnv = "NSELF_BINARY_SIGNING_PUBKEY_HEX"

// Keys is the committed set of release public keys (hex Ed25519). None is
// committed yet, so only the operator-pinned key can verify.
var Keys = []string{}

// Require refuses an install unless sigHex is a valid Ed25519 signature, by a
// trusted key, over the digest text sum.
func Require(sum, sigHex string) error {
	if sum == "" {
		return fmt.Errorf("--binary-url requires --binary-sha256: the signature covers that digest")
	}
	sigHex = strings.TrimSpace(sigHex)
	if sigHex == "" {
		return fmt.Errorf("--binary-url requires --binary-signature: an unsigned binary is refused")
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("--binary-signature must be a %d-byte hex Ed25519 signature", ed25519.SignatureSize)
	}
	keys := append([]string{}, Keys...)
	if k := strings.TrimSpace(os.Getenv(KeyEnv)); k != "" {
		keys = append(keys, k)
	}
	for _, k := range keys {
		pub, err := hex.DecodeString(strings.TrimSpace(k))
		if err != nil || len(pub) != ed25519.PublicKeySize {
			continue
		}
		if ed25519.Verify(pub, []byte(sum), sig) {
			return nil
		}
	}
	return fmt.Errorf("--binary-signature is not a valid signature of the digest by a trusted release key (pin one with %s)", KeyEnv)
}
