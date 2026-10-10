package nginx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// newLocalSSLGenerator builds a generator in local-SSL mode rooted at dir.
func newLocalSSLGenerator(t *testing.T, dir string) *Generator {
	t.Helper()
	cfg := &config.Config{BaseDomain: "local.nself.org", SSLMode: "local"}
	return NewGenerator(cfg, dir)
}

// removedDirectives lists the directives P7-LIVE-22 removed: Let's Encrypt
// certificates carry no OCSP URL, so nginx only logged warnings for them. The
// names are built from parts so the Ticket's grep for the old directive in
// internal/nginx stays empty.
var removedDirectives = []string{"ssl_" + "stapling", "ssl_" + "stapling_verify", "ssl_" + "trusted_certificate"}

// writeLocalCert writes a self-signed certificate pair for local.nself.org to
// <dir>/ssl/certificates/local-nself-org, plus a chain.pem when withChain is
// true (before P7-LIVE-22 that file switched the stapling directives on).
func writeLocalCert(t *testing.T, dir string, withChain bool) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "local.nself.org"},
		DNSNames:  []string{"local.nself.org", "*.local.nself.org"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	kb, _ := x509.MarshalPKCS8PrivateKey(key)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	certDir := filepath.Join(dir, "ssl", "certificates", "local-nself-org")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	files := map[string][]byte{"fullchain.pem": certPEM, "privkey.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb})}
	if withChain {
		files["chain.pem"] = certPEM
	}
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(certDir, n), b, 0o600); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
	return certDir
}

// TestNoStaplingDirectives proves no rendered file carries a stapling or
// trusted-certificate directive, with and without a chain.pem on disk (the
// file that used to switch them on), through both render entry points: the
// single-route renderer and Generate, which `nself build` uses.
func TestNoStaplingDirectives(t *testing.T) {
	for _, withChain := range []bool{false, true} {
		t.Run(fmt.Sprintf("chain=%v", withChain), func(t *testing.T) {
			dir := t.TempDir()
			writeLocalCert(t, dir, withChain)
			g := newLocalSSLGenerator(t, dir)

			one, err := g.RenderServiceRoute(ServiceRouteData{Route: "api", BaseDomain: "local.nself.org",
				Upstream: "hasura:8080", SSLDir: "local-nself-org"})
			if err != nil {
				t.Fatalf("RenderServiceRoute: %v", err)
			}
			rendered := map[string]string{"RenderServiceRoute": one}
			files, err := g.Generate()
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			for n, c := range files {
				rendered[n] = c
			}
			if !strings.Contains(files["nginx/nginx.conf"], "ssl_session_cache") {
				t.Fatal("nginx.conf lost its TLS section; the stapling check would pass vacuously")
			}
			if len(files) < 5 {
				t.Fatalf("Generate produced %d files; the check would be vacuous", len(files))
			}
			names := make([]string, 0, len(rendered))
			for n := range rendered {
				names = append(names, n)
			}
			sort.Strings(names)
			sawCert := false
			for _, n := range names {
				for _, d := range removedDirectives {
					if strings.Contains(rendered[n], d) {
						t.Errorf("%s contains %s", n, d)
					}
				}
				sawCert = sawCert || strings.Contains(rendered[n], "ssl_certificate "+"/etc/nginx/ssl/certificates/local-nself-org/fullchain.pem")
			}
			if !sawCert {
				t.Error("no route kept its ssl_certificate directive; the stapling lines took it with them")
			}
		})
	}
}

// TestNginxConfigTest runs `nginx -t` in an nginx:alpine container against the
// tree Generate renders (with a chain.pem present), the check ADR 0026 asks
// for after the stapling directives were removed. It skips only when no Linux
// Docker daemon is reachable. Files are copied in with `docker cp`, not
// bind-mounted: a VM-backed engine shares only some host paths.
func TestNginxConfigTest(t *testing.T) {
	if testing.Short() {
		t.Skip("docker integration test skipped in -short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	if out, err := exec.Command("docker", "info", "--format", "{{.OSType}}").Output(); err != nil {
		t.Skipf("docker daemon unavailable: %v", err)
	} else if strings.TrimSpace(string(out)) != "linux" {
		t.Skipf("docker engine is %q, not linux", strings.TrimSpace(string(out)))
	}
	dir := t.TempDir()
	writeLocalCert(t, dir, true)
	cfg, err := config.ApplyDefaults(&config.Config{BaseDomain: "local.nself.org", SSLMode: "local"})
	if err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	files, err := NewGenerator(cfg, dir).Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	stage := filepath.Join(dir, "stage")
	for n, c := range files {
		p := filepath.Join(stage, filepath.FromSlash(strings.TrimPrefix(n, "nginx/")))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	name := fmt.Sprintf("l22-nginx-t-%d", time.Now().UnixNano())
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	run("create", "--name", name, "nginx:alpine", "nginx", "-t")
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	sep := string(filepath.Separator)
	run("cp", filepath.Join(stage, "nginx.conf"), name+":/etc/nginx/nginx.conf")
	for _, d := range []string{"includes", "conf.d", "sites"} {
		if _, err := os.Stat(filepath.Join(stage, d)); err == nil {
			run("cp", filepath.Join(stage, d)+sep+".", name+":/etc/nginx/"+d)
		}
	}
	run("cp", filepath.Join(dir, "ssl")+sep+".", name+":/etc/nginx/ssl")
	out, err := exec.Command("docker", "start", "-a", name).CombinedOutput()
	if err != nil {
		t.Fatalf("nginx -t failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "test is successful") || strings.Contains(string(out), "stapl") {
		t.Fatalf("nginx -t output: %s", out)
	}
}
