package doctor

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
)

func TestDoctorE126StaleEntitlement(t *testing.T) {
	installDoctorPlugin(t, true)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := license.PingKeys
	license.PingKeys = []license.PingKey{{ID: "synthetic-doctor", Public: pub}}
	t.Cleanup(func() { license.PingKeys = old })
	now := time.Unix(1800000000, 0)
	compattest.Both(t, func(t *testing.T) {
		for _, tc := range []struct {
			name string
			age  time.Duration
			want bool
		}{
			{"fresh", -time.Hour, false},
			{"exact seven days", 7 * 24 * time.Hour, false},
			{"stale", 7*24*time.Hour + time.Second, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, ".nself", "state", "entitlement.jwt")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "kid": "synthetic-doctor"})
				payload, _ := json.Marshal(map[string]any{"exp": now.Add(-tc.age).Unix(), "iss": "ping.nself.org", "aud": "nself-cli"})
				enc := base64.RawURLEncoding
				unsigned := enc.EncodeToString(header) + "." + enc.EncodeToString(payload)
				token := unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
				if err := os.WriteFile(path, []byte(token), 0600); err != nil {
					t.Fatal(err)
				}
				got := entitlementFileChecks(dir, now)
				want := tc.want && compat.V15()
				if (len(got) > 0) != want {
					t.Fatalf("findings = %+v, want stale %v", got, want)
				}
				if want && (got[0].Status != "warn" || !strings.Contains(got[0].Message, "E126")) {
					t.Fatalf("missing E126 warning: %+v", got)
				}
			})
		}
	})
}

func TestDoctorEntitlementMissingAndInvalid(t *testing.T) {
	compattest.Set(t, true)
	installDoctorPlugin(t, true)
	dir := t.TempDir()
	if got := entitlementFileChecks(dir, time.Now()); len(got) != 1 {
		t.Fatalf("missing file: %+v", got)
	}
	path := filepath.Join(dir, ".nself", "state", "entitlement.jwt")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("obviously-fake-invalid-jwt"), 0600); err != nil {
		t.Fatal(err)
	}
	got := entitlementFileChecks(dir, time.Now())
	if len(got) != 1 || got[0].Status != "warn" || strings.Contains(got[0].Message, "E126") {
		t.Fatalf("invalid token should warn without trusting exp: %+v", got)
	}
}

func installDoctorPlugin(t *testing.T, licensed bool) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	if !licensed {
		return
	}
	if err := os.Mkdir(filepath.Join(dir, "fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture", "plugin.json"), []byte(`{"license":"licensed"}`), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorEntitlementInstallationGate(t *testing.T) {
	compattest.Set(t, true)
	for _, licensed := range []bool{false, true} {
		for _, shape := range []string{"missing", "empty", "directory", "invalid"} {
			t.Run(shape+"/"+map[bool]string{false: "free", true: "licensed"}[licensed], func(t *testing.T) {
				installDoctorPlugin(t, licensed)
				dir := t.TempDir()
				path := filepath.Join(dir, ".nself", "state", "entitlement.jwt")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				switch shape {
				case "directory":
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				case "empty", "invalid":
					data := []byte{}
					if shape == "invalid" {
						data = []byte("invalid")
					}
					if err := os.WriteFile(path, data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if got := entitlementFileChecks(dir, time.Now()); (len(got) > 0) != licensed {
					t.Fatalf("findings=%+v licensed=%v", got, licensed)
				}
			})
		}
	}
}
