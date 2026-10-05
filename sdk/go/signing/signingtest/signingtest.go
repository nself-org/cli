// Package signingtest gives tests a fresh in-memory key. Keys are generated per
// call and never written anywhere; no production key belongs here.
package signingtest

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// NewKey generates a key of purpose p with the derived id, and a Signer for it.
func NewKey(t testing.TB, p signing.Purpose) (signing.Key, signing.Signer) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("signingtest: generate key: %v", err)
	}
	id := signing.KeyID(p, pub)
	return signing.Key{ID: id, Purpose: p, Public: pub}, signing.NewEd25519Signer(id, priv)
}
