//go:build nself_devkeys

package license

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
)

// extraKeys in a dev build: each source (ldflags var, LICENSE_PUBLIC_KEY_OVERRIDE)
// adds one key when, and only when, it decodes to a usable Ed25519 key.
func TestDevExtraKeysSourcesAndRejects(t *testing.T) {
	good, _ := testKeypairHex(t)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")
	orig := licensePubKeyHex
	t.Cleanup(func() { licensePubKeyHex = orig })

	licensePubKeyHex = ""
	if got := extraKeys(); len(got) != 0 {
		t.Fatalf("no source set: want no extra keys, got %d", len(got))
	}

	licensePubKeyHex = good
	got := extraKeys()
	if len(got) != 1 || got[0].ID != 1 || got[0].KID != "dev" || hex.EncodeToString(got[0].Key) != good {
		t.Fatalf("ldflags key: got %+v", got)
	}

	licensePubKeyHex = ""
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", good)
	if got := extraKeys(); len(got) != 1 {
		t.Fatalf("override key: want 1 extra key, got %d", len(got))
	}

	licensePubKeyHex = good
	if got := extraKeys(); len(got) != 2 {
		t.Fatalf("both sources: want 2 extra keys, got %d", len(got))
	}

	for name, bad := range map[string]string{
		"not hex":        "not-valid-hex",
		"odd length":     good[:63],
		"too short":      "abcd",
		"too long":       good + "00",
		"all zero":       strings.Repeat("00", ed25519.PublicKeySize),
		"whitespace":     " " + good,
		"uppercase junk": strings.Repeat("G", 64),
	} {
		licensePubKeyHex = ""
		t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", bad)
		if got := extraKeys(); len(got) != 0 {
			t.Errorf("override %s: an unusable key must not be trusted, got %d", name, len(got))
		}
		t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")
		licensePubKeyHex = bad
		if got := extraKeys(); len(got) != 0 {
			t.Errorf("ldflags %s: an unusable key must not be trusted, got %d", name, len(got))
		}
	}
}

// The warning names the source that supplied the key, once per process.
func TestDevExtraKeysWarningNamesSource(t *testing.T) {
	good, _ := testKeypairHex(t)
	orig := licensePubKeyHex
	t.Cleanup(func() { licensePubKeyHex = orig })
	licensePubKeyHex = ""
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", good)
	resetDevKeyWarning()
	out := captureStderr(t, func() { extraKeys(); extraKeys() })
	if strings.Count(out, "warning:") != 1 || !strings.Contains(out, "LICENSE_PUBLIC_KEY_OVERRIDE") {
		t.Errorf("want one warning naming the override, got %q", out)
	}
	licensePubKeyHex = good
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")
	resetDevKeyWarning()
	out = captureStderr(t, func() { extraKeys() })
	if !strings.Contains(out, "ldflags licensePubKeyHex") {
		t.Errorf("want a warning naming the ldflags key, got %q", out)
	}
}

// A dev build still refuses a response signed by an untrusted key and accepts
// one signed by the dev key: both security directions.
func TestDevKeyAcceptsOnlyItsOwnSignatures(t *testing.T) {
	pubHex, priv := testKeypairHex(t)
	_, stranger := testKeypairHex(t)
	orig := licensePubKeyHex
	t.Cleanup(func() { licensePubKeyHex = orig })
	licensePubKeyHex = pubHex
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")
	body := []byte(`{"valid":true}`)
	if err := verifyResponseSig(body, signHex(priv, body)); err != nil {
		t.Errorf("dev key signature must verify: %v", err)
	}
	if err := verifyResponseSig(body, signHex(stranger, body)); err == nil {
		t.Error("a signature by an unknown key must not verify")
	}
	if err := verifyResponseSig([]byte(`{"valid":false}`), signHex(priv, body)); err == nil {
		t.Error("a signature over a different body must not verify")
	}
}
