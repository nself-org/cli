package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/nginxtopo"
)

// TestGenerate_RealBuildPathRootsSSLCertificateAtMountPath is the regression
// guard for the bug found live on production 2026-09-21: `nself build` wrote
// nginx/sites/*.conf files whose ssl_certificate directives read
// "/certificates/<dir>/fullchain.pem" instead of
// "/etc/nginx/ssl/certificates/<dir>/fullchain.pem", so nginx refused to
// start with "cannot load certificate /certificates/...: BIO_new_file()
// failed".
//
// Cause: service.conf.tmpl renders "{{.SSLBasePath}}/certificates/...", and
// RenderServiceRoute set ServiceRouteData.SSLBasePath correctly, but the
// bulk generateAllRoutes() loop in routes.go — the code path `nself build`
// actually uses to write every nginx/sites/*.conf file — never set it, so it
// rendered empty. This test drives the real Generate() entrypoint (not
// RenderServiceRoute) so it exercises the same loop `nself build` runs
// through, the way TestGenerate_RealBuildPathEmitsTrustedChainWhenPresent
// does for the sibling HasTrustedChain gap.
func TestGenerate_RealBuildPathRootsSSLCertificateAtMountPath(t *testing.T) {
	dir := t.TempDir()
	certDir := filepath.Join(dir, "ssl", "certificates", "local-nself-org")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("-----BEGIN CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatalf("write fullchain.pem: %v", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("-----BEGIN PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatalf("write privkey.pem: %v", err)
	}

	g := newLocalSSLGenerator(t, dir)
	files, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wantPrefix := "ssl_certificate " + nginxtopo.NginxSSLContainerPath + "/certificates/"
	badPrefix := "ssl_certificate /certificates/"

	sawSSLCert := false
	for name, content := range files {
		if !strings.HasPrefix(name, "nginx/sites/") {
			continue
		}
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "ssl_certificate ") {
				continue
			}
			sawSSLCert = true
			if strings.HasPrefix(trimmed, badPrefix) {
				t.Errorf("%s: ssl_certificate rooted at bare %q, nginx will refuse to start — line: %q", name, "/certificates/", trimmed)
			}
			if !strings.HasPrefix(trimmed, wantPrefix) {
				t.Errorf("%s: ssl_certificate not rooted at the ssl mount path %q — line: %q", name, wantPrefix, trimmed)
			}
		}
	}
	if !sawSSLCert {
		t.Fatal("no ssl_certificate directive found in any generated nginx/sites/*.conf — test fixture is not exercising SSL mode")
	}
}
