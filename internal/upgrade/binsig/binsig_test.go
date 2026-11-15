package binsig

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
)

// signingFixture creates ephemeral keys and signs the digest text, not its bytes.
func signingFixture(t *testing.T) (string, string, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := strings.Repeat("ab", 32)
	return hex.EncodeToString(pub), sum, hex.EncodeToString(ed25519.Sign(priv, []byte(sum)))
}

func trustedKeys(t *testing.T, keys ...string) {
	t.Helper()
	old := Keys
	t.Cleanup(func() { Keys = old })
	Keys = keys
	t.Setenv(KeyEnv, "")
}

func requireError(t *testing.T, sum, sig, message string) {
	t.Helper()
	if err := Require(sum, sig); err == nil || !strings.Contains(err.Error(), message) {
		t.Fatalf("Require: got %v, want error containing %q", err, message)
	}
}

func TestRequireAcceptsPinnedKey(t *testing.T) {
	trustedKeys(t)
	key, sum, sig := signingFixture(t)
	for _, space := range []string{"", " \t\n"} {
		t.Setenv(KeyEnv, space+key+space)
		if err := Require(sum, space+sig+space); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRequireAcceptsCommittedKey(t *testing.T) {
	key, sum, sig := signingFixture(t)
	trustedKeys(t, key)
	if err := Require(sum, sig); err != nil {
		t.Fatal(err)
	}
}

func TestRequireRefusesMissingInputs(t *testing.T) {
	trustedKeys(t)
	_, sum, sig := signingFixture(t)
	requireError(t, "", sig, "--binary-sha256")
	requireError(t, sum, " \n", "--binary-signature")
}

func TestRequireRefusesMalformedSignature(t *testing.T) {
	trustedKeys(t)
	_, sum, _ := signingFixture(t)
	for _, sig := range []string{"not-hex", "ab"} {
		requireError(t, sum, sig, "64-byte hex Ed25519 signature")
	}
}

func TestRequireRefusesUntrustedSignature(t *testing.T) {
	trustedKeys(t)
	key, sum, sig := signingFixture(t)
	_, _, otherSig := signingFixture(t)
	t.Setenv(KeyEnv, key)
	requireError(t, sum, otherSig, KeyEnv)
	// Require accepts any nonempty digest shape; these fail because the exact
	// text differs from the signed text, including case and whitespace.
	for _, changed := range []string{"another digest", sum + " ", strings.ToUpper(sum)} {
		requireError(t, changed, sig, KeyEnv)
	}
	t.Setenv(KeyEnv, "")
	requireError(t, sum, sig, KeyEnv)
}

func TestRequireIgnoresMalformedKeys(t *testing.T) {
	key, sum, sig := signingFixture(t)
	trustedKeys(t)
	for _, bad := range []string{"not-hex", "ab"} {
		t.Setenv(KeyEnv, bad)
		requireError(t, sum, sig, KeyEnv)
	}
	Keys = []string{"not-hex", "ab"}
	t.Setenv(KeyEnv, key)
	if err := Require(sum, sig); err != nil {
		t.Fatal(err)
	}
	Keys = []string{"not-hex", "ab", " \t" + key + "\n"}
	t.Setenv(KeyEnv, "")
	if err := Require(sum, sig); err != nil {
		t.Fatal(err)
	}
}
