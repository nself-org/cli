package license

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

const testLicenseKey = "nself_plus_testkeytestkeytestkey"

// pingStub serves a /license/validate reply shaped like routes/license-validate.ts,
// signed with a test key the way ping signs: header = hex(ed25519(body)), jwt =
// EdDSA token whose sub is sha256(licence key).
type pingStub struct {
	priv ed25519.PrivateKey
	body []byte
}

// newPingStub starts the stub, points PingKeys at its key and the CLI at the
// stub URL and a temp cache file. Nothing leaves the machine.
func newPingStub(t *testing.T, mutate func(map[string]any)) *pingStub {
	t.Helper()
	priv := useTestPingKey(t)
	claims := goodClaims()
	claims["sub"] = HashKey(testLicenseKey)
	m := map[string]any{
		"valid": true, "tier": "plus", "product": "plus", "products_covered": []string{"claw"},
		"plugins_allowed": []string{"ai", "bundle:chat"}, "features": []string{},
		"can_install_plugin": true, "upgrade_required": false,
		"expires_at": "2027-01-01T00:00:00.000Z", "key_type": "product",
		"jwt": signEdDSA(t, priv, goodHeader(), claims), "jwt_kid": "t1", "jwt_expires_at": 1700086400,
	}
	if mutate != nil {
		mutate(m)
	}
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &pingStub{priv: priv, body: body}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(priv, body)))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LICENSE_PING_URL", srv.URL)
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
	return s
}

// validateAndRead runs the real ValidateFull against the stub and returns the cache it wrote.
func validateAndRead(t *testing.T) (*ValidationResult, *CacheEntry) {
	t.Helper()
	res, err := ValidateFull(context.Background(), testLicenseKey)
	if err != nil || !res.Valid {
		t.Fatalf("ValidateFull: %+v, %v", res, err)
	}
	entry, err := ReadCache()
	if err != nil || entry == nil {
		t.Fatalf("ReadCache: %v, %v", entry, err)
	}
	return res, entry
}

func TestCacheSignedBodyVerifies(t *testing.T) {
	newPingStub(t, nil)
	compattest.Set(t, true)
	res, entry := validateAndRead(t)
	if entry.RawBody == "" || entry.BodySig == "" || entry.JWT == "" || entry.JWTKid != "t1" {
		t.Fatalf("cache must store the raw body, header signature and jwt: %+v", entry)
	}
	if !entry.VerifySignature() {
		t.Fatal("entry written from a signed response must verify against the committed key set")
	}
	if got := strings.Join(entry.PluginsAllowed, ","); got != "ai,bundle:chat" {
		t.Errorf("cache PluginsAllowed = %q, want the plugins_allowed list", got)
	}
	if got := strings.Join(res.Plugins, ","); got != "ai,bundle:chat" {
		t.Errorf("result Plugins = %q, want the plugins_allowed list", got)
	}
	// FetchedAt is local and unsigned: moving it must not change the verdict.
	entry.FetchedAt += 12345
	if !entry.VerifySignature() {
		t.Error("fetched_at is not part of the signature")
	}
}

func TestCacheSignedTamperFails(t *testing.T) {
	newPingStub(t, nil)
	compattest.Set(t, true)
	_, good := validateAndRead(t)
	if !good.VerifySignature() {
		t.Fatal("precondition: untouched entry verifies")
	}
	clone := func() *CacheEntry {
		c := *good
		c.PluginsAllowed = append([]string(nil), good.PluginsAllowed...)
		return &c
	}
	flip := func(s string, i int) string {
		b := []byte(s)
		b[i] ^= 0x01
		return string(b)
	}
	cases := map[string]func(c *CacheEntry){
		"one byte of the body":    func(c *CacheEntry) { c.RawBody = strings.Replace(c.RawBody, `"plus"`, `"pluz"`, 1) },
		"first body byte":         func(c *CacheEntry) { c.RawBody = flip(c.RawBody, 0) },
		"signature bit":           func(c *CacheEntry) { c.BodySig = flip(c.BodySig, 3) },
		"signature truncated":     func(c *CacheEntry) { c.BodySig = c.BodySig[:len(c.BodySig)-2] },
		"signature not hex":       func(c *CacheEntry) { c.BodySig = "zz" + c.BodySig[2:] },
		"signature removed":       func(c *CacheEntry) { c.BodySig = "" },
		"body removed":            func(c *CacheEntry) { c.RawBody = "" },
		"tier copy raised":        func(c *CacheEntry) { c.Tier = "owner" },
		"plugin injected":         func(c *CacheEntry) { c.PluginsAllowed = append(c.PluginsAllowed, "voice") },
		"plugins emptied":         func(c *CacheEntry) { c.PluginsAllowed = nil },
		"expiry extended":         func(c *CacheEntry) { c.ExpiresAt += 86400 * 365 },
		"key hash swapped":        func(c *CacheEntry) { c.KeyHash = HashKey("nself_owner_someoneelseskeyvalue") },
		"jwt replaced by garbage": func(c *CacheEntry) { c.JWT = "a.b.c" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			c := clone()
			mut(c)
			if c.VerifySignature() {
				t.Errorf("tampered entry (%s) verified", name)
			}
		})
	}

	t.Run("wrong key", func(t *testing.T) {
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		orig := PingKeys
		PingKeys = []PingKey{{ID: "t1", Public: pub}}
		defer func() { PingKeys = orig }()
		if good.VerifySignature() {
			t.Error("entry verified against a key that did not sign it")
		}
	})
	t.Run("no committed keys", func(t *testing.T) {
		orig := PingKeys
		PingKeys = nil
		defer func() { PingKeys = orig }()
		if good.VerifySignature() {
			t.Error("an empty key set must never verify")
		}
	})
	t.Run("body signed but invalid", func(t *testing.T) {
		newPingStub(t, func(m map[string]any) { m["valid"] = false })
		if _, err := ValidateFull(context.Background(), testLicenseKey); err != nil {
			t.Fatal(err)
		}
		entry, _ := ReadCache()
		if entry.VerifySignature() {
			t.Error("a signed valid:false body must not verify as a licence")
		}
	})
}

// A cache file from before this change (no raw body, legacy signature fields)
// is read, and is unverified in v1.5: nothing in it was signed by the server.
func TestCacheLegacyEntryUnverifiedInV15(t *testing.T) {
	legacy := &CacheEntry{KeyHash: HashKey(testLicenseKey), Tier: "plus", PluginsAllowed: []string{"ai"},
		FetchedAt: 1, ExpiresAt: 2, Signature: strings.Repeat("ab", 64), SignatureKeyID: 1}
	compattest.Both(t, func(t *testing.T) {
		useTestPingKey(t)
		if legacy.VerifySignature() {
			t.Error("a legacy entry with a bogus signature must not verify in either mode")
		}
	})
}
