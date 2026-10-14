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
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
	"github.com/spf13/cobra"
)

func TestLicenseSetOutsideProjectThenBuild(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	for i := 1; i <= 10; i++ {
		t.Setenv("NSELF_LICENSE_KEY_"+strconv.Itoa(i), "")
	}
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
	outside := t.TempDir()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })
	key := "nself_chat_abcdefghijklmnopqrstuvwxyz1234"
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	oldKeys := license.PingKeys
	license.PingKeys = []license.PingKey{{ID: "synthetic", Public: pub}}
	t.Cleanup(func() { license.PingKeys = oldKeys })
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"EdDSA","kid":"synthetic"}`))
	payload, _ := json.Marshal(map[string]any{"sub": license.HashKey(key), "plugins": []string{"chat"}, "iss": "ping.nself.org", "aud": "nself-cli", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()})
	unsigned := header + "." + enc.EncodeToString(payload)
	jwt := unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
	body, _ := json.Marshal(map[string]any{"valid": true, "jwt": jwt, "tier": "plus", "plugins_allowed": []string{"chat"}})
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(priv, body)))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := licenseSetCmd.RunE(cmd, []string{key}); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("outside set validates once: %d", requests)
	}
	if _, err := os.Stat(entitlement.Path(outside)); !os.IsNotExist(err) {
		t.Fatalf("outside project wrote runtime file: %v", err)
	}
	server.Close() // The next build must also work from the captured JWT offline.
	plugins := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", plugins)
	p := filepath.Join(plugins, "chat")
	if err := os.MkdirAll(p, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "plugin.json"), []byte(`{"manifest_version":2,"license":"licensed","name":"chat"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "docker-compose.plugin.yml"), []byte("services:\n  chat:\n    image: fixture:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if _, err := build.DiscoverPluginComposeFiles(project, plugins); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(entitlement.Path(project))
	if err != nil || string(data) != jwt {
		t.Fatalf("next build did not obtain entitlement: %v", err)
	}
}
