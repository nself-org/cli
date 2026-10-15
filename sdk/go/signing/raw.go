package signing

import (
	"crypto/ed25519"
	"fmt"
)

// VerifyEd25519 verifies a raw signature with a caller-selected trusted key.
// Inputs: public key, exact signed message and raw signature bytes.
// Outputs: nil on success, ErrMalformed or ErrBadSignature on rejection.
// Constraints: rejects small-order keys and malformed lengths without panicking.
// Legacy protocols retain responsibility for key selection, claims and encoding;
// use Verifier for purpose, scope, revocation and key validity enforcement.
func VerifyEd25519(pub ed25519.PublicKey, msg, sig []byte) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: public key length %d", ErrMalformed, len(pub))
	}
	if hasSmallOrder(pub) {
		return fmt.Errorf("%w: public key is a small-order point", ErrMalformed)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature length %d", ErrMalformed, len(sig))
	}
	if !ed25519.Verify(pub, msg, sig) {
		return ErrBadSignature
	}
	return nil
}
