package entitlement_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
)

func signedFixture(t *testing.T, key string, plugins []string, exp time.Time) (*license.CacheEntry, ed25519.PrivateKey) {
	t.Helper()
	seedVerifiedCache(t)
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", key)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	license.PingKeys = []license.PingKey{{ID: "synthetic", Public: pub}}
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"EdDSA","kid":"synthetic"}`))
	payload, _ := json.Marshal(map[string]any{"sub": license.HashKey(key), "plugins": plugins, "iss": "ping.nself.org", "aud": "nself-cli", "exp": exp.Unix(), "iat": time.Now().Unix()})
	unsigned := header + "." + enc.EncodeToString(payload)
	jwt := unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
	body, _ := json.Marshal(map[string]any{"valid": true, "tier": "plus", "plugins_allowed": plugins, "jwt": jwt})
	entry := &license.CacheEntry{KeyHash: license.HashKey(key), Tier: "plus", PluginsAllowed: plugins, JWT: jwt, RawBody: string(body), BodySig: hex.EncodeToString(ed25519.Sign(priv, body)), FetchedAt: time.Now().Add(-48 * time.Hour).Unix()}
	if err := license.WriteCache(entry); err != nil {
		t.Fatal(err)
	}
	return entry, priv
}

func installLicensed(t *testing.T, name string) {
	t.Helper()
	p := filepath.Join(os.Getenv("NSELF_PLUGIN_DIR"), name)
	if err := os.MkdirAll(p, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "plugin.json"), []byte(`{"manifest_version":2,"license":"licensed","name":"`+name+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshRevalidatesOnline(t *testing.T) {
	compattest.Set(t, true)
	entry, priv := signedFixture(t, "synthetic-license-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	parts := strings.Split(entry.JWT, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	claims["exp"] = time.Now().Add(2 * time.Hour).Unix()
	payload, _ = json.Marshal(claims)
	unsigned := parts[0] + "." + base64.RawURLEncoding.EncodeToString(payload)
	newJWT := unsigned + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
	newBody, _ := json.Marshal(map[string]any{"valid": true, "tier": "plus", "plugins_allowed": []string{"chat"}, "jwt": newJWT})
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(priv, newBody)))
		_, _ = w.Write(newBody)
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(entitlement.Path(dir))
	cache, _ := license.ReadCache()
	if count != 1 || string(data) != newJWT || cache.FetchedAt <= entry.FetchedAt {
		t.Fatalf("renewal count=%d, cache=%v", count, cache)
	}
}

func TestRefreshExpiredBeyondGraceE125(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "synthetic-license-key", []string{"*"}, time.Now().Add(-31*24*time.Hour))
	dir := t.TempDir()
	err := entitlement.Refresh(context.Background(), dir)
	if errs.ExitCodeFor(err) != 3 || !strings.Contains(err.Error(), "entitlement grace ended") {
		t.Fatalf("expired grace: %v", err)
	}
	data, _ := os.ReadFile(entitlement.Path(dir))
	if string(data) != entry.JWT {
		t.Fatal("expired signed token must still publish")
	}
}

func TestRefreshTwoKeysTwoBundles(t *testing.T) {
	compattest.Set(t, true)
	signedFixture(t, "synthetic-chat-key", []string{"chat"}, time.Now().Add(time.Hour))
	t.Setenv("NSELF_LICENSE_KEY_1", "synthetic-claw-key")
	installLicensed(t, "chat")
	installLicensed(t, "claw")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	err := entitlement.Refresh(context.Background(), t.TempDir())
	if errs.ExitCodeFor(err) != 3 || !strings.Contains(err.Error(), "claw") {
		t.Fatalf("uncovered bundle: %v", err)
	}
	if calls != 2 {
		t.Fatalf("want once per configured key, got %d", calls)
	}
}

func TestRefreshForStartWarnsAndContinues(t *testing.T) {
	compattest.Set(t, true)
	signedFixture(t, "synthetic-chat-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "claw")
	var warnings bytes.Buffer
	entitlement.RefreshForStart(context.Background(), t.TempDir(), &warnings)
	if !strings.Contains(warnings.String(), "E125") {
		t.Fatalf("missing warning: %s", warnings.String())
	}
}

func TestStartMayContinueWrappedAutoBuildE125(t *testing.T) {
	var warnings bytes.Buffer
	err := fmt.Errorf("auto-build failed: %w", errs.New("E125", "token missing"))
	if !entitlement.StartMayContinue(err, &warnings) || !strings.Contains(warnings.String(), "E125") {
		t.Fatal("auto-build entitlement failure aborted free services")
	}
	if entitlement.StartMayContinue(fmt.Errorf("build disk failure"), &warnings) {
		t.Fatal("unrelated build failure suppressed")
	}
	if !entitlement.StartMayContinue(nil, &warnings) {
		t.Fatal("success refused")
	}
}

func TestRefreshOfflineRetainsGoodFileAndRemovedKeyClears(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "synthetic-license-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(os.Getenv("LICENSE_CACHE_PATH")); err != nil {
		t.Fatal(err)
	}
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(entitlement.Path(dir))
	if string(data) != entry.JWT {
		t.Fatal("offline lost verified token")
	}
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(entitlement.Path(dir))
	if len(data) != 0 {
		t.Fatal("removed key retained token")
	}
}
