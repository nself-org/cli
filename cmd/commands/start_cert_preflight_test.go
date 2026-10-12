package commands

// start_cert_preflight_test.go: D-0503. The `nself start` certificate check
// reads <project>/ssl/certificates/<dir>/fullchain.pem, where nself writes
// the pair, and stays quiet for a healthy cert; the old join without ssl/
// warned on every start.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTestCert writes a synthetic self-signed fullchain.pem valid for days.
func writeTestCert(t *testing.T, path string, days int) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "local.example.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Duration(days) * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStartCertPath(t *testing.T) {
	for domain, want := range map[string]string{
		"local.example.test": filepath.Join("proj", "ssl", "certificates", "local-example-test", "fullchain.pem"),
		"":                   filepath.Join("proj", "ssl", "certificates", "localhost", "fullchain.pem"),
	} {
		if got := startCertPath("proj", domain); got != want {
			t.Errorf("startCertPath(%q) = %s, want %s", domain, got, want)
		}
	}
}

func TestCertExpiryWarning(t *testing.T) {
	project := t.TempDir()
	certPath := startCertPath(project, "local.example.test")
	writeTestCert(t, certPath, 90)

	// The pair nself wrote is found: no warning on a healthy cert.
	if msg := certExpiryWarning(certPath, true); msg != "" {
		t.Errorf("healthy cert warned: %q", msg)
	}
	// Positive control: the pre-fix join (no ssl/) misses that same pair and warns.
	oldPath := filepath.Join(project, "certificates", "local-example-test", "fullchain.pem")
	if msg := certExpiryWarning(oldPath, true); !strings.Contains(msg, "no such file") {
		t.Errorf("old path: %q, want the missing-file warning", msg)
	}

	soon := filepath.Join(t.TempDir(), "fullchain.pem")
	writeTestCert(t, soon, 10)
	if msg := certExpiryWarning(soon, true); !strings.Contains(msg, "renew soon") {
		t.Errorf("expiring cert: %q, want a renew warning", msg)
	}

	missing := filepath.Join(t.TempDir(), "fullchain.pem")
	if msg := certExpiryWarning(missing, false); msg != "" {
		t.Errorf("missing cert, mode without local certs: %q, want no warning", msg)
	}
	if msg := certExpiryWarning(missing, true); !strings.Contains(msg, "TLS certificate issue") {
		t.Errorf("missing cert, local mode: %q, want a warning", msg)
	}
}
