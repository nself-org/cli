package commands

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
)

// TestBundleInfoPluginsAllowed runs the real licence flow against a stub ping
// that signs a response in the shape routes/license-validate.ts sends (free
// tier, plugins_allowed only, no `plugins`), then asks `bundle info` for the
// chat bundle's licence status. v1.4 prints what the base binary prints (not
// included: the legacy field is always empty); v1.5 sees the real list.
func TestBundleInfoPluginsAllowed(t *testing.T) {
	b, ok := bundle.Get("chat")
	if !ok || len(b.Plugins) == 0 {
		t.Skip("chat bundle has no plugins in this build")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	orig := license.PingKeys
	license.PingKeys = []license.PingKey{{ID: "1", Public: pub}}
	t.Cleanup(func() { license.PingKeys = orig })

	const key = "nself_chat_bundleinfotestkey0000000000"
	// ping's licence JWT: sub = sha256(key), 24 h window, signed by the same key.
	now := time.Now().Unix()
	seg := func(v any) string {
		j, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(j)
	}
	signing := seg(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "1"}) + "." + seg(map[string]any{
		"sub": license.HashKey(key), "tier": "free", "plugins": []string{b.Plugins[0]},
		"iat": now, "exp": now + 86400, "iss": "ping.nself.org", "aud": "nself-cli"})
	jwt := signing + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(signing)))
	body := `{"valid":true,"tier":"free","plugins_allowed":["` + b.Plugins[0] + `"],"expires_at":"2099-01-01T00:00:00.000Z","jwt":"` + jwt + `","jwt_kid":"1"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(priv, []byte(body))))
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))

	compattest.Both(t, func(t *testing.T) {
		res, err := license.ValidateFull(context.Background(), key)
		if err != nil || !res.Valid {
			t.Fatalf("ValidateFull: %+v, %v", res, err)
		}
		got := resolveBundleLicenseStatus("chat")
		wantActive := strings.Contains(t.Name(), "v1.5")
		if gotActive := strings.HasPrefix(got, "active"); gotActive != wantActive {
			t.Errorf("status %q: active=%v, want %v", got, gotActive, wantActive)
		}
	})
}
