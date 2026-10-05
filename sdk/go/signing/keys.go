package signing

import (
	"crypto/ed25519"
	"crypto/sha256"
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

// checkKey validates the static shape of a key.
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
	return nil
}

// clip bounds attacker-controlled text placed in an error message.
func clip(s string) string {
	if len(s) > 64 {
		return s[:64] + "..."
	}
	return s
}
