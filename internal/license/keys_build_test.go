package license

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
)

// TestKeysCommittedNonZero: the committed ping key set is never empty, each key
// decodes to 32 bytes that are not all zero, and kids are unique and non-empty.
func TestKeysCommittedNonZero(t *testing.T) {
	if len(PingKeys) == 0 {
		t.Fatal("PingKeys is empty")
	}
	seen := map[string]bool{}
	for _, k := range PingKeys {
		if k.ID == "" || seen[k.ID] {
			t.Errorf("kid %q is empty or duplicated", k.ID)
		}
		seen[k.ID] = true
		if len(k.Public) != ed25519.PublicKeySize {
			t.Errorf("kid %s: key is %d bytes, want %d", k.ID, len(k.Public), ed25519.PublicKeySize)
		}
		if bytes.Equal(k.Public, make([]byte, ed25519.PublicKeySize)) {
			t.Errorf("kid %s: key is all zero", k.ID)
		}
	}
}

func TestPubKeyFromHex(t *testing.T) {
	for _, bad := range []string{"", "zz", "abcd", "00"} {
		if pubKeyFromHex(bad) != nil {
			t.Errorf("pubKeyFromHex(%q) should be nil", bad)
		}
	}
	if pubKeyFromHex("0ac4c2d7ec30d23bf2be55775e7170bd24a7544bad036d0d19f2a4deb516e00c") == nil {
		t.Error("valid key rejected")
	}
}

// compatMode15 reports whether the running subtest selected v1.5 (compattest).
func compatMode15() bool { return compat.V15() }

func testKeypairHex(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(pub), priv
}

func signHex(priv ed25519.PrivateKey, msg []byte) string {
	return hex.EncodeToString(ed25519.Sign(priv, msg))
}

// setPingKeys replaces the committed key set for one test and restores it.
func setPingKeys(t *testing.T, keys ...PingKey) {
	t.Helper()
	orig := PingKeys
	PingKeys = keys
	t.Cleanup(func() { PingKeys = orig })
}

// useTestKey makes the key behind pubHex the only key that verifies ping
// signatures for this test (the seam that replaces LICENSE_PUBLIC_KEY_OVERRIDE
// in default builds, which ignore it). An undecodable value leaves no key.
func useTestKey(t *testing.T, pubHex string) {
	t.Helper()
	b, _ := hex.DecodeString(pubHex)
	setPingKeys(t, PingKey{ID: "1", Public: b})
}

// TestReleaseBuildIgnoresOverride: in a default build (no tags: what goreleaser
// and go install produce) neither LICENSE_PUBLIC_KEY_OVERRIDE nor a key in the
// environment adds a verification key, and a signature by that key fails.
func TestReleaseBuildIgnoresOverride(t *testing.T) {
	if devKeysBuild {
		t.Skip("default builds only; the nself_devkeys build honours the override")
	}
	pubHex, priv := testKeypairHex(t)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", pubHex)
	got := GetPublicKeys()
	if len(got) != len(PingKeys) {
		t.Fatalf("GetPublicKeys returned %d keys, want exactly the %d committed", len(got), len(PingKeys))
	}
	for i, k := range got {
		if !bytes.Equal(k.Key, PingKeys[i].Public) {
			t.Errorf("key %d is not the committed key", i)
		}
	}
	body := []byte(`{"valid":true}`)
	if err := verifyResponseSig(body, signHex(priv, body)); err == nil {
		t.Error("a signature by the override key verified in a default build")
	}
	if IsZeroPubKey() {
		t.Error("the committed set must never read as zero")
	}
}

// TestZeroOrAbsentKeyFailsClosed: with no usable key nothing verifies, and an
// all-zero committed key is dropped rather than trusted.
func TestZeroOrAbsentKeyFailsClosed(t *testing.T) {
	_, priv := testKeypairHex(t)
	body := []byte(`{"valid":true}`)
	sig := signHex(priv, body)
	for name, keys := range map[string][]PingKey{
		"absent":   nil,
		"nil key":  {{ID: "1"}},
		"all zero": {{ID: "1", Public: make([]byte, ed25519.PublicKeySize)}},
		"short":    {{ID: "1", Public: []byte{1, 2, 3}}},
	} {
		t.Run(name, func(t *testing.T) {
			setPingKeys(t, keys...)
			if !IsZeroPubKey() || len(GetPublicKeys()) != 0 {
				t.Error("no usable key must read as zero")
			}
			if err := verifyResponseSig(body, sig); err == nil {
				t.Error("verification succeeded with no usable key")
			}
			if (&CacheEntry{RawBody: string(body), BodySig: sig}).VerifySignature() {
				t.Error("cache verified with no usable key")
			}
		})
	}
}

// signingServer wraps h so every reply carries X-NSelf-License-Sig, signed with
// a fresh test key that is also installed as the only committed key for the
// test. It stands in for ping_api in tests that exercise a successful remote
// validation; nothing leaves the machine.
func signingServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	pub, priv := testKeypairHex(t)
	useTestKey(t, pub)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		body := rec.Body.Bytes()
		w.Header().Set("X-NSelf-License-Sig", signHex(priv, body))
		w.WriteHeader(rec.Code)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDevBuildHonoursOverride: under -tags nself_devkeys the override key is
// added to the committed keys, a signature by it verifies, and exactly one
// warning is printed however often keys are read.
func TestDevBuildHonoursOverride(t *testing.T) {
	if !devKeysBuild {
		t.Skip("needs -tags nself_devkeys")
	}
	pubHex, priv := testKeypairHex(t)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", pubHex)
	resetDevKeyWarning()
	out := captureStderr(t, func() {
		for i := 0; i < 3; i++ {
			if got := GetPublicKeys(); len(got) != len(PingKeys)+1 {
				t.Errorf("GetPublicKeys returned %d keys, want committed+1", len(got))
			}
		}
	})
	if n := strings.Count(out, "warning:"); n != 1 {
		t.Errorf("want exactly one warning, got %d: %q", n, out)
	}
	body := []byte(`{"valid":true}`)
	if err := verifyResponseSig(body, signHex(priv, body)); err != nil {
		t.Errorf("a signature by the override key should verify in a dev build: %v", err)
	}
}
