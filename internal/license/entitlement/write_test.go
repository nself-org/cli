package entitlement_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
)

func seedVerifiedCache(t *testing.T) *license.CacheEntry {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "synthetic-license-key")
	for i := 1; i <= 10; i++ {
		t.Setenv("NSELF_LICENSE_KEY_"+strconv.Itoa(i), "")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := license.PingKeys
	license.PingKeys = []license.PingKey{{ID: "synthetic", Public: pub}}
	t.Cleanup(func() { license.PingKeys = old })
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
	enc := base64.RawURLEncoding
	hdr := enc.EncodeToString([]byte(`{"alg":"EdDSA","kid":"synthetic"}`))
	claims, _ := json.Marshal(map[string]any{
		"sub": license.HashKey("synthetic-license-key"), "tier": "plus", "plugins": []string{"*"},
		"iss": "ping.nself.org", "aud": "nself-cli", "iat": time.Now().Unix(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	unsigned := hdr + "." + enc.EncodeToString(claims)
	token := unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
	body, _ := json.Marshal(map[string]any{
		"valid": true, "tier": "plus", "plugins_allowed": []string{"*"}, "jwt": token,
	})
	entry := &license.CacheEntry{
		KeyHash: license.HashKey("synthetic-license-key"), Tier: "plus", PluginsAllowed: []string{"*"},
		RawBody: string(body), BodySig: hex.EncodeToString(ed25519.Sign(priv, body)), JWT: token,
	}
	if err := license.WriteCache(entry); err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestEntitlementRefreshWrites(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		entry := seedVerifiedCache(t)
		dir := t.TempDir()
		if err := entitlement.Refresh(context.Background(), dir); err != nil {
			t.Fatal(err)
		}
		path := entitlement.Path(dir)
		if !compat.V15() {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("v1.4 created runtime file: %v", err)
			}
			return
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != entry.JWT {
			t.Fatalf("runtime file must equal cached JWT: %v", err)
		}
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && st.Mode().Perm() != 0600 {
			t.Fatalf("runtime file mode must be 0600: %v", err)
		}
		entry.RawBody += "tampered"
		if err := license.WriteCache(entry); err != nil {
			t.Fatal(err)
		}
		if err := entitlement.Refresh(context.Background(), dir); err != nil {
			t.Fatalf("valid runtime JWT survives tampered cache: %v", err)
		}
		data, err = os.ReadFile(path)
		if err != nil || string(data) != entry.JWT {
			t.Fatalf("tampered cache must retain verified runtime JWT: %v", err)
		}
	})
}

func TestBuildE125UnderV15(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		// No licence key from the host: a stored or env key would renew a real JWT online.
		t.Setenv("HOME", t.TempDir())
		t.Setenv("USERPROFILE", t.TempDir())
		t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
		t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "")
		t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "missing.json"))
		// Missing-token tests must not renew a licence configured on the CI host.
		t.Setenv("HOME", t.TempDir())
		t.Setenv("USERPROFILE", t.TempDir())
		t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
		for i := 1; i <= 10; i++ {
			t.Setenv("NSELF_LICENSE_KEY_"+strconv.Itoa(i), "")
		}
		dir, plugins := t.TempDir(), t.TempDir()
		installed := filepath.Join(plugins, "fixture-licensed")
		if err := os.MkdirAll(installed, 0700); err != nil {
			t.Fatal(err)
		}
		fixture, err := os.ReadFile("../../plugin/manifestv2/testdata/v2/licensed.json")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(installed, "plugin.json"), fixture, 0600); err != nil {
			t.Fatal(err)
		}
		compose := []byte("services:\n  fixture:\n    image: fixture:latest\n    environment:\n      PORT: '9999'\n    networks:\n      - default\n")
		if err := os.WriteFile(filepath.Join(installed, "docker-compose.plugin.yml"), compose, 0600); err != nil {
			t.Fatal(err)
		}
		_, err = build.DiscoverPluginComposeFiles(dir, plugins)
		if !compat.V15() {
			if err != nil {
				t.Fatal(err)
			}
			return
		}
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E125" || errs.ExitCodeFor(err) != 3 {
			t.Fatalf("build missing JWT must fail E125 exit 3: %v", err)
		}
		data, readErr := os.ReadFile(entitlement.Path(dir))
		if readErr != nil || len(data) != 0 {
			t.Fatalf("build failure must leave empty runtime token: %v", readErr)
		}
	})
}
