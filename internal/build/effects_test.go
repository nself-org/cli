package build

// effects_test.go — ties planSSL to the ssl.Generator skip predicates
// (P7-LIVE-21 review S4): both call ssl.NeedsCerts and ssl.CertPairValid, so
// plan mode cannot drift from write mode on when certificates are generated.
// Inputs: temp ssl dirs, self-signed certificate pairs. Outputs: pass/fail.

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
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ssl"
)

// writeCertPair writes a self-signed fullchain.pem and a privkey.pem to dir.
func writeCertPair(t *testing.T, dir string, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "plan-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), pemBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem"), []byte("KEY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func planSSLCfg(t *testing.T, mode string) *config.Config {
	t.Helper()
	cfg, err := config.ApplyDefaults(&config.Config{BaseDomain: "example.test", SSLMode: mode, ProjectName: "x", Env: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func hasEffect(fx *planEffects, kind string) bool {
	for _, e := range fx.Recorded() {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

// Modes that skip local certificates: generator and plan agree, and neither
// records a certificate effect.
func TestPlanSSLSkipModesMatchGenerator(t *testing.T) {
	for _, mode := range []string{"letsencrypt", "custom", "none"} {
		t.Run(mode, func(t *testing.T) {
			if ssl.NeedsCerts(mode) {
				t.Fatalf("NeedsCerts(%q) = true", mode)
			}
			cfg := planSSLCfg(t, mode)
			fx := &planEffects{}
			res, domains := planSSL(fx, cfg, t.TempDir(), false)
			if res.Count != 0 || domains != nil || len(fx.Recorded()) != 0 {
				t.Errorf("plan: count %d domains %v effects %v", res.Count, domains, fx.Recorded())
			}
			out := t.TempDir()
			got, err := ssl.NewGenerator(cfg).GenerateWithResult(out)
			if err != nil || got.Count != 0 {
				t.Errorf("generator: count %d err %v", got.Count, err)
			}
			if entries, _ := os.ReadDir(out); len(entries) != 0 {
				t.Errorf("generator wrote into a skipped mode: %v", entries)
			}
		})
	}
}

// A fresh certificate pair makes the plan record no certificates effect; a
// near-expiry, missing or unreadable pair makes it record one, exactly when
// ssl.CertPairValid says the generator would regenerate.
func TestPlanSSLCertEffectFollowsCertPairValid(t *testing.T) {
	cases := []struct {
		name     string
		notAfter time.Time
		write    bool
	}{
		{"fresh", time.Now().Add(90 * 24 * time.Hour), true},
		{"near expiry", time.Now().Add(10 * 24 * time.Hour), true},
		{"expired", time.Now().Add(-24 * time.Hour), true},
		{"absent", time.Time{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := planSSLCfg(t, "local")
			sslDir := t.TempDir()
			certDir := filepath.Join(sslDir, "certificates", ssl.DomainToDirName(cfg.BaseDomain))
			if c.write {
				writeCertPair(t, certDir, c.notAfter)
			}
			fx := &planEffects{}
			planSSL(fx, cfg, sslDir, false)
			wantEffect := !ssl.CertPairValid(certDir)
			if got := hasEffect(fx, EffectCertificates); got != wantEffect {
				t.Errorf("certificates effect = %v, CertPairValid says regenerate = %v", got, wantEffect)
			}
			if c.name == "fresh" && wantEffect {
				t.Error("a 90-day pair must not be regenerated")
			}
			if c.name != "fresh" && !wantEffect {
				t.Errorf("%s pair must be regenerated", c.name)
			}
		})
	}
}
