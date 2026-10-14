package entitlement

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
)

func TestCachedTokenRequiresPositiveSignedExpiry(t *testing.T) {
	compattest.Set(t, true)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := license.PingKeys
	license.PingKeys = []license.PingKey{{ID: "synthetic", Public: pub}}
	t.Cleanup(func() { license.PingKeys = old })
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "valid-key")
	for _, exp := range []int64{-1, 0, 1} {
		enc := base64.RawURLEncoding
		claims, _ := json.Marshal(map[string]any{"sub": license.HashKey("valid-key"), "iss": "ping.nself.org", "aud": "nself-cli", "exp": exp})
		unsigned := enc.EncodeToString([]byte(`{"alg":"EdDSA","kid":"synthetic"}`)) + "." + enc.EncodeToString(claims)
		token := unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
		body, _ := json.Marshal(map[string]any{"valid": true, "jwt": token})
		entry := &license.CacheEntry{KeyHash: license.HashKey("valid-key"), JWT: token, RawBody: string(body), BodySig: hex.EncodeToString(ed25519.Sign(priv, body))}
		if err := license.WriteCache(entry); err != nil {
			t.Fatal(err)
		}
		got, err := cachedToken()
		if exp <= 0 && (got != "" || err == nil || !strings.Contains(err.Error(), "signed expiry")) {
			t.Fatalf("nonpositive expiry %d accepted: token=%q err=%v", exp, got, err)
		}
		if exp > 0 && (err != nil || got != token) {
			t.Fatalf("positive signed expiry rejected: %v", err)
		}
	}
}
