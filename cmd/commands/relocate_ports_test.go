package commands

// Purpose: TestBreakoutAutoAddFixturePorts (P7-CANON-13, readiness R1) runs the
// v1.5 auto-add against the real merged ports (ci, functions, infra, plugin-dev)
// built and signed by scripts/ci/ports-fixture-registry.sh, so the relocation is
// proven against the binaries users get, not stubs.
// Inputs: NSELF_CANON_PORTS_REGISTRY, the output directory of that script.
// Constraints: skips only when that variable is unset; the generated key replaces
// the key table for the test (restored by t.Cleanup); HOME and the project are
// throwaway; the server is loopback only.

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

func TestBreakoutAutoAddFixturePorts(t *testing.T) {
	dir := os.Getenv("NSELF_CANON_PORTS_REGISTRY")
	if dir == "" {
		t.Skip("NSELF_CANON_PORTS_REGISTRY is unset (run scripts/ci/ports-fixture-registry.sh first)")
	}
	compattest.Set(t, true)
	keyText, err := os.ReadFile(filepath.Join(dir, "key.pub"))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyText)))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		t.Fatalf("key.pub: %v (%d bytes)", err, len(pub))
	}
	signaturetest.UseKeys(t, []signature.Key{{ID: signing.KeyID(signing.PurposePlugins, pub), Tier: signature.TierFree, PublicKey: pub}}, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})

	registry, err := os.ReadFile(filepath.Join(dir, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case len(parts) == 3 && parts[0] == "plugins" && parts[2] == "tarball" && !strings.ContainsAny(parts[1], "/.\\"):
			http.ServeFile(w, r, filepath.Join(dir, "tarballs", parts[1]+".tar.gz"))
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			http.NotFound(w, r) // a per-platform probe stays on the fixture server
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(bytes.ReplaceAll(registry, []byte("@BASE@"), []byte(srv.URL)))
		}
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_DIR", "")
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_SKIP_SBOM_CHECK", "1")
	t.Setenv("NSELF_PLUGIN_REQUIRE_CHECKSUM", "")
	t.Setenv(envNoAutoAdd, "")
	project := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	warn := &bytes.Buffer{}
	prev := relocateWarnOut
	relocateWarnOut = warn
	t.Cleanup(func() { relocateWarnOut = prev })

	for _, tc := range []struct {
		slug string
		argv []string
	}{
		{"ci", []string{"ci", "--help"}},
		{"functions", []string{"functions", "--help"}},
		{"infra", []string{"infra", "server", "--help"}},
		{"plugin-dev", []string{"plugin-dev", "init", "--help"}},
	} {
		warn.Reset()
		handled, err := relocateBreakout(tc.argv[0], tc.argv[1:])
		if !handled || err != nil {
			t.Fatalf("nself %s: handled=%v err=%v", strings.Join(tc.argv, " "), handled, err)
		}
		if n := strings.Count(warn.String(), "\n"); n != 1 || !strings.Contains(warn.String(), "installed "+tc.slug+"@") {
			t.Fatalf("nself %s: want one warning naming %s, got %q", strings.Join(tc.argv, " "), tc.slug, warn.String())
		}
	}

	// help --json now lists each command as a plugin command.
	args := os.Args
	os.Args = []string{"nself", "help"} // the mount skips an invocation whose first token is a flag
	t.Cleanup(func() { os.Args = args })
	reattachRealTree()
	undo := prepareTree(RootCmd, true, true)
	t.Cleanup(func() { undo(); reattachRealTree() })
	reg, err := BuildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"ci", "functions", "infra", "plugin-dev"} {
		found := false
		for _, c := range reg.Commands {
			if c.Path == "nself "+slug {
				found = true
				if c.Canon != canon.CanonPlugin || c.Plugin == nil || *c.Plugin != slug {
					t.Errorf("nself %s: canon %q plugin %v, want canon plugin from %s", slug, c.Canon, c.Plugin, slug)
				}
			}
		}
		if !found {
			t.Errorf("help --json does not list nself %s after the auto-add", slug)
		}
	}
}
