package build

// plugin_nginx_ssldir_test.go — D-0478 end to end in nginx: the generated
// tree plus the real backup plugin snippet, rendered by the build's own
// injection, passes `nginx -t` in nginx:alpine. Before the fix nginx failed
// with: unknown "ssl_dir" variable.

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
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx"
)

// writeSyntheticCert writes a throwaway self-signed pair where
// internal/ssl.Generator would put the local.nself.org certificate.
func writeSyntheticCert(t *testing.T, dir string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "local.nself.org"},
		DNSNames:  []string{"local.nself.org", "*.local.nself.org"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certDir := filepath.Join(dir, "ssl", "certificates", "local-nself-org")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, b := range map[string][]byte{
		"fullchain.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		"privkey.pem":   pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}),
	} {
		if err := os.WriteFile(filepath.Join(certDir, n), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPluginSnippetNginxConfigTest(t *testing.T) {
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
	writeSyntheticCert(t, dir)
	pluginDir := filepath.Join(t.TempDir(), "plugins")
	if err := os.MkdirAll(filepath.Join(pluginDir, "backup", "nginx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "backup", "nginx", "backup.conf"), []byte(backupSnippet), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.ApplyDefaults(&config.Config{ProjectName: "d0478", BaseDomain: "local.nself.org", SSLMode: "local"})
	if err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	files, err := nginx.NewGenerator(cfg, dir).Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for n, c := range files {
		p := filepath.Join(dir, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := InjectPluginNginxRoutes(dir, pluginDir, cfg); err != nil || n != 1 {
		t.Fatalf("InjectPluginNginxRoutes = %d, %v", n, err)
	}

	name := fmt.Sprintf("d0478-nginx-t-%d", time.Now().UnixNano())
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("docker %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	run("create", "--name", name, "nginx:alpine", "nginx", "-t")
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	sep := string(filepath.Separator)
	stage := filepath.Join(dir, "nginx")
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
	if !strings.Contains(string(out), "test is successful") {
		t.Fatalf("nginx -t output: %s", out)
	}
}
