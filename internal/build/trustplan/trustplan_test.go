package trustplan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalDevStep: only local development certificates under the project and
// the mkcert CA named by fingerprint (or as a new CA) are bindable steps.
func TestLocalDevStep(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "proj")
	certs := filepath.Join(proj, "ssl", "certificates", "local-nself-org")
	local := CertGenerate + "localhost, 127.0.0.1, ::1, local.nself.org, *.local.nself.org, api.local.nself.org"
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rootCA.pem"), []byte("ROOT CA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	named := TrustDetail(root)
	if !strings.HasPrefix(named, trustDetailInstall) || named == trustDetailNew {
		t.Fatalf("an existing CA is not named by fingerprint: %q", named)
	}
	if got := TrustDetail(t.TempDir()); got != trustDetailNew {
		t.Fatalf("a missing CA: %q", got)
	}
	for _, tc := range []struct {
		name, kind, target, detail string
		want                       bool
	}{
		{"local certificate", certificates, certs, local, true},
		{"copied local certificate", certificates, certs, CertCopy + "localhost, app.localhost", true},
		{"public domain", certificates, certs, local + ", example.com", false},
		{"non-loopback address", certificates, certs, CertGenerate + "localhost, 10.0.0.5", false},
		{"outside the project", certificates, filepath.Join(t.TempDir(), "ssl", "certificates", "x"), local, false},
		{"nested target", certificates, filepath.Join(certs, "deeper"), local, false},
		{"unknown detail", certificates, certs, "issue something for localhost", false},
		{"named CA", trustStore, Target, named, true},
		{"new CA", trustStore, Target, trustDetailNew, true},
		{"unnamed CA", trustStore, Target, "install the mkcert CA when it is not yet trusted", false},
		{"short fingerprint", trustStore, Target, trustDetailInstall + "abcd" + trustDetailSuffix, false},
		{"other trust target", trustStore, "corporate CA", named, false},
		{"hosts", "hosts", "/etc/hosts", "ensure entries: localhost", false},
	} {
		if got := LocalDevStep(tc.kind, tc.target, tc.detail, proj); got != tc.want {
			t.Errorf("%s: LocalDevStep = %v, want %v", tc.name, got, tc.want)
		}
	}
}
