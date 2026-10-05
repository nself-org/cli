package signing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"
)

// Purpose names what a key may sign. A verifier accepts exactly one.
type Purpose string

// The purposes (Epic CACHE section Contracts, cap:sdk.go-signing).
const (
	PurposePlugins   Purpose = "plugins"
	PurposeAgent     Purpose = "agent"
	PurposeCIRelease Purpose = "ci-release"
	PurposeCINode    Purpose = "ci-node"
	PurposeCIAudit   Purpose = "ci-audit"
)

// Valid reports whether p is one of the five defined purposes.
func (p Purpose) Valid() bool {
	switch p {
	case PurposePlugins, PurposeAgent, PurposeCIRelease, PurposeCINode, PurposeCIAudit:
		return true
	}
	return false
}

// Key is a public verification key with its trust attributes.
type Key struct {
	ID      string
	Purpose Purpose
	// Scope narrows a purpose (the plugin tier, for example). Empty is no scope.
	Scope  string
	Public ed25519.PublicKey
	// NotBefore (inclusive) and NotAfter (exclusive) bound validity; zero is
	// unbounded.
	NotBefore time.Time
	NotAfter  time.Time
}

// KeyID derives the canonical key id: "<purpose>-<first 16 hex of sha256(pub)>".
func KeyID(p Purpose, pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return string(p) + "-" + hex.EncodeToString(sum[:])[:16]
}

const maxKeyIDLen = 128

// validKeyID reports whether id is 1..128 characters of [A-Za-z0-9._-].
func validKeyID(id string) bool {
	if len(id) == 0 || len(id) > maxKeyIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if !isIDChar(id[i]) {
			return false
		}
	}
	return true
}

// isIDChar reports whether c is in [A-Za-z0-9._-].
func isIDChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '.' || c == '_' || c == '-'
}

// smallOrder lists the encodings of the small-order points of edwards25519
// and the non-canonical aliases of y=0 and y=1, with the sign bit cleared. It
// is the blocklist libsodium uses (ge25519_has_small_order in
// crypto_sign/ed25519/ref10), which compares with bit 255 ignored:
// y=0 (order 4), y=1 (identity), y=p-1 (order 2), the two order-8 points, and
// y=p, y=p+1 (non-canonical aliases of y=0 and y=1).
var smallOrder = [...][32]byte{
	{},
	{0x01},
	{0x26, 0xe8, 0x95, 0x8f, 0xc2, 0xb2, 0x27, 0xb0, 0x45, 0xc3, 0xf4, 0x89, 0xf2, 0xef, 0x98, 0xf0,
		0xd5, 0xdf, 0xac, 0x05, 0xd3, 0xc6, 0x33, 0x39, 0xb1, 0x38, 0x02, 0x88, 0x6d, 0x53, 0xfc, 0x05},
	{0xc7, 0x17, 0x6a, 0x70, 0x3d, 0x4d, 0xd8, 0x4f, 0xba, 0x3c, 0x0b, 0x76, 0x0d, 0x10, 0x67, 0x0f,
		0x2a, 0x20, 0x53, 0xfa, 0x2c, 0x39, 0xcc, 0xc6, 0x4e, 0xc7, 0xfd, 0x77, 0x92, 0xac, 0x03, 0x7a},
	nearP(0xec),
	nearP(0xed),
	nearP(0xee),
}

// nearP is first byte b, then 30 bytes 0xff, then 0x7f (y = p-1, p, p+1).
func nearP(b byte) [32]byte {
	var o [32]byte
	for i := range o {
		o[i] = 0xff
	}
	o[0] = b
	o[31] = 0x7f
	return o
}

// hasSmallOrder reports whether pub encodes a small-order point (any key whose
// signatures verify over every message). The sign bit is ignored. A 32-byte
// input is required; any other length reports true (refused).
func hasSmallOrder(pub []byte) bool {
	if len(pub) != ed25519.PublicKeySize {
		return true
	}
	var c [32]byte
	copy(c[:], pub)
	c[31] &= 0x7f
	for i := range smallOrder {
		if c == smallOrder[i] {
			return true
		}
	}
	return false
}

// checkKey validates a key's shape: legal id and purpose, 32-byte public key
// of large order, and an id that is exactly KeyID(purpose, pub), so a key
// cannot be listed under a chosen id or another purpose's id.
func checkKey(k Key) error {
	if !validKeyID(k.ID) {
		return fmt.Errorf("%w: key id %q", ErrMalformed, clip(k.ID))
	}
	if !k.Purpose.Valid() {
		return fmt.Errorf("%w: key %q has purpose %q", ErrMalformed, k.ID, clip(string(k.Purpose)))
	}
	if len(k.Public) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: key %q public key length %d", ErrMalformed, k.ID, len(k.Public))
	}
	if hasSmallOrder(k.Public) {
		return fmt.Errorf("%w: key %q is a small-order point", ErrMalformed, k.ID)
	}
	if subtle.ConstantTimeCompare([]byte(k.ID), []byte(KeyID(k.Purpose, k.Public))) != 1 {
		return fmt.Errorf("%w: key id %q is not KeyID(%s, key)", ErrMalformed, k.ID, k.Purpose)
	}
	return nil
}

// clip bounds attacker-controlled text placed in an error message.
func clip(s string) string {
	if len(s) > 64 {
		return s[:64] + "..."
	}
	return s
}
