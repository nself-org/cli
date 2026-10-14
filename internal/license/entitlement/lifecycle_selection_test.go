package entitlement_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
)

func replacementEntry(t *testing.T, priv ed25519.PrivateKey, key string, plugins []string, exp time.Time) *license.CacheEntry {
	t.Helper()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"EdDSA","kid":"synthetic"}`))
	payload, _ := json.Marshal(map[string]any{"sub": license.HashKey(key), "plugins": plugins, "iss": "ping.nself.org", "aud": "nself-cli", "exp": exp.Unix(), "iat": time.Now().Unix()})
	unsigned := header + "." + enc.EncodeToString(payload)
	jwt := unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
	body, _ := json.Marshal(map[string]any{"valid": true, "tier": "plus", "plugins_allowed": plugins, "jwt": jwt})
	return &license.CacheEntry{KeyHash: license.HashKey(key), JWT: jwt, Tier: "plus", PluginsAllowed: plugins, RawBody: string(body), BodySig: hex.EncodeToString(ed25519.Sign(priv, body)), FetchedAt: time.Now().Unix()}
}

func putRuntimeToken(t *testing.T, dir, token string) {
	t.Helper()
	path := entitlement.Path(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshRejectedKeyKeepsOtherCoveringToken(t *testing.T) {
	for _, validFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "reject-first", true: "valid-first"}[validFirst], func(t *testing.T) {
			compattest.Set(t, true)
			entry, _ := signedFixture(t, "valid-key", []string{"chat"}, time.Now().Add(time.Hour))
			if validFirst {
				t.Setenv("NSELF_LICENSE_KEY_1", "rejected-key")
			} else {
				t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "rejected-key")
				t.Setenv("NSELF_LICENSE_KEY_1", "valid-key")
			}
			installLicensed(t, "chat")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Key string `json:"license_key"`
				}
				_ = json.NewDecoder(r.Body).Decode(&request)
				if request.Key == "rejected-key" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("X-NSelf-License-Sig", entry.BodySig)
				_, _ = w.Write([]byte(entry.RawBody))
			}))
			defer server.Close()
			t.Setenv("NSELF_PING_API_URL", server.URL)
			dir := t.TempDir()
			if err := entitlement.Refresh(context.Background(), dir); err != nil {
				t.Fatalf("other valid key must survive: %v", err)
			}
			data, _ := os.ReadFile(entitlement.Path(dir))
			if string(data) != entry.JWT {
				t.Fatal("other key lost its covering entitlement")
			}
		})
	}
}

func TestRefreshOfflinePrefersLaterCoveringExpiry(t *testing.T) {
	for _, expanded := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-plugins", true: "new-plugin"}[expanded], func(t *testing.T) {
			compattest.Set(t, true)
			old, priv := signedFixture(t, "valid-key", []string{"chat"}, time.Now().Add(time.Hour))
			installLicensed(t, "chat")
			plugins := []string{"chat"}
			if expanded {
				plugins = append(plugins, "claw")
				installLicensed(t, "claw")
			}
			newer := replacementEntry(t, priv, "valid-key", plugins, time.Now().Add(2*time.Hour))
			if err := license.WriteCache(newer); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			putRuntimeToken(t, dir, old.JWT)
			if err := entitlement.Refresh(context.Background(), dir); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(entitlement.Path(dir))
			if string(data) != newer.JWT {
				t.Fatal("older file defeated newer covering cache token")
			}
		})
	}
}

func TestRefreshPortalForbiddenRetainsToken(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "valid-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html>Sign in to Wi-Fi</html>"))
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	putRuntimeToken(t, dir, entry.JWT)
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(entitlement.Path(dir))
	if string(data) != entry.JWT {
		t.Fatal("captive portal cleared signed entitlement")
	}
}

func TestRefreshHardLinkRepairMessage(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "valid-key", []string{"chat"}, time.Now().Add(time.Hour))
	dir := t.TempDir()
	putRuntimeToken(t, dir, "stale-token")
	if err := os.Link(entitlement.Path(dir), filepath.Join(dir, "alias")); err != nil {
		t.Skip(err)
	}
	before, err := os.Stat(entitlement.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	err = entitlement.Refresh(context.Background(), dir)
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("Windows refresh must support existing regular files: %v", err)
		}
		after, statErr := os.Stat(entitlement.Path(dir))
		data, readErr := os.ReadFile(filepath.Join(dir, "alias"))
		if statErr != nil || readErr != nil || !os.SameFile(before, after) || string(data) != entry.JWT {
			t.Fatalf("Windows refresh lost inode or alias token: stat=%v read=%v", statErr, readErr)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), "remove the entitlement file") {
		t.Fatalf("missing repair instruction: %v", err)
	}
}

func TestRefreshRejectedKeyKeepsOtherOfflineToken(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "valid-key", []string{"chat"}, time.Now().Add(time.Hour))
	t.Setenv("NSELF_LICENSE_KEY_1", "rejected-key")
	installLicensed(t, "chat")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Key string `json:"license_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Key == "rejected-key" {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	putRuntimeToken(t, dir, entry.JWT)
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(entitlement.Path(dir))
	if string(data) != entry.JWT {
		t.Fatal("another key's offline token was cleared")
	}
}

func TestRefreshPingForbiddenClearsToken(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "valid-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"valid":false,"error":"License rejected"}`))
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	putRuntimeToken(t, dir, entry.JWT)
	if err := entitlement.Refresh(context.Background(), dir); err == nil {
		t.Fatal("rejected key accepted")
	}
	data, _ := os.ReadFile(entitlement.Path(dir))
	if len(data) != 0 {
		t.Fatal("rejected subject retained")
	}
}

func TestRefreshKeepsRenewalBeforeLaterSignedRejection(t *testing.T) {
	compattest.Set(t, true)
	entry, priv := signedFixture(t, "valid-key", []string{"chat"}, time.Now().Add(time.Hour))
	t.Setenv("NSELF_LICENSE_KEY_1", "rejected-key")
	installLicensed(t, "chat")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Key string `json:"license_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		body := entry.RawBody
		if request.Key == "rejected-key" {
			body = `{"valid":false,"reason":"revoked"}`
		}
		w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(priv, []byte(body))))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(entitlement.Path(dir))
	if string(data) != entry.JWT {
		t.Fatal("later signed rejection overwrote another key's renewal")
	}
}
