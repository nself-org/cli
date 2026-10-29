package plugin

// env_bypass_attack_test.go — P7-PLUG-87 review H1 regression. A cloned
// project's env files set every signature-bypass knob and point the registry
// at an attacker; the registry install must still be refused (E107 unsigned,
// E121 bad signature, E122 unknown key), nothing may be written, and the
// attacker's registry is never contacted. Fails on the pre-fix loader, which
// exported every .env key into the process environment.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

// attackEnv is the hostile env file: the H1 exploit knobs plus every other
// protected key, with the registry and service URLs pointed at attacker.
func attackEnv(attacker string) string {
	lines := []string{"ENV=dev", "PROJECT_NAME=victim"}
	for _, k := range config.ProtectedEnvKeys() {
		v := "1"
		if strings.HasSuffix(k, "_URL") || k == "NSELF_PLUGIN_REGISTRY" {
			v = attacker
		}
		if k == pluginRequireChecksumEnv {
			v = "0"
		}
		if k == "NSELF_PLUGIN_INSTALL_TIER" {
			v = "pro"
		}
		lines = append(lines, k+"="+v)
	}
	return strings.Join(lines, "\n") + "\n"
}

// attackProject writes the hostile env into every cascade layer and returns
// the project dir and a counter of requests that reached the attacker.
func attackProject(t *testing.T) (string, *int32) {
	t.Helper()
	var hits int32
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(evil.Close)
	dir := t.TempDir()
	for _, name := range []string{".env", ".env.dev", ".env.secrets", ".env.local"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(attackEnv(evil.URL)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range append(config.ProtectedEnvKeys(), "ENV", "PROJECT_NAME") {
		if _, set := os.LookupEnv(k); !set {
			t.Setenv(k, "") // registers the restore if a regression exports it
			_ = os.Unsetenv(k)
		}
	}
	return dir, &hits
}

func TestProjectEnvCannotDisableSignatureEnforcement(t *testing.T) {
	tb := sigTarball(t)
	sum := sha256Hex(tb)
	scenarios := []struct {
		name, want string
		extra      func(t *testing.T) map[string]any
	}{
		{"unsigned", "E107", func(t *testing.T) map[string]any {
			key, _ := signaturetest.NewKey(t, signature.TierFree)
			signaturetest.UseKeys(t, []signature.Key{key}, nil)
			return nil
		}},
		{"bad signature", "E121", func(t *testing.T) map[string]any {
			key, signer := signaturetest.NewKey(t, signature.TierFree)
			signaturetest.UseKeys(t, []signature.Key{key}, nil)
			return relMap(signaturetest.Sign(t, signer, sigPlugin, "1.0.0", sha256Hex([]byte("other bytes"))))
		}},
		{"unknown key", "E122", func(t *testing.T) map[string]any {
			key, _ := signaturetest.NewKey(t, signature.TierFree)
			_, stranger := signaturetest.NewKey(t, signature.TierFree)
			signaturetest.UseKeys(t, []signature.Key{key}, nil)
			return relMap(signaturetest.Sign(t, stranger, sigPlugin, "1.0.0", sum))
		}},
	}
	for _, sc := range scenarios {
		for _, shell := range []bool{false, true} {
			name := sc.name + "/project env only"
			if shell {
				name = sc.name + "/operator shell skip + --force"
			}
			t.Run(name, func(t *testing.T) {
				compattest.Both(t, func(t *testing.T) {
					signaturetest.UseFloor(t, map[string]map[string]string{signature.TierFree: {sigPlugin: "1.0.0"}})
					extra := sc.extra(t)
					downloads := sigServer(t, tb, sum, extra) // the operator's registry
					registry := os.Getenv("NSELF_PLUGIN_REGISTRY")
					dir, evilHits := attackProject(t)
					if shell {
						t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
					}
					loaded, err := config.Load(dir)
					if err != nil {
						t.Fatal(err)
					}
					if loaded.Env != "dev" {
						t.Fatalf("Env = %q: the project .env must still choose the cascade", loaded.Env)
					}
					if got := os.Getenv("NSELF_PLUGIN_REGISTRY"); got != registry {
						t.Fatalf("registry = %q after load, want the operator's %q", got, registry)
					}
					// --force on the command line: still no bypass for a registry install.
					ctx := WithSignatureBypassAcknowledged(context.Background(), true)
					pluginDir := t.TempDir()
					var ierr error
					captureStderr(t, func() {
						ierr = installLocked(ctx, &config.Config{ProjectName: "t", Env: loaded.Env}, sigPlugin, pluginDir)
					})
					if codeOf(ierr) != sc.want {
						t.Fatalf("want %s, got %v", sc.want, ierr)
					}
					if atomic.LoadInt32(downloads) == 0 {
						t.Fatal("never reached the download: the test does not exercise the signature step")
					}
					if !dirEmpty(t, pluginDir) {
						t.Fatal("a file was written although the signature failed")
					}
					if n := atomic.LoadInt32(evilHits); n != 0 {
						t.Fatalf("the attacker's registry was contacted %d time(s)", n)
					}
				})
			})
		}
	}
}
