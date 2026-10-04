package ssl

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestClassifyCert pins the 21/7 day thresholds (D14).
func TestClassifyCert(t *testing.T) {
	tests := []struct {
		days int
		want CertStatus
	}{
		{90, CertOK}, {30, CertOK}, {21, CertOK},
		{20, CertWarn}, {7, CertWarn},
		{6, CertFail}, {0, CertFail}, {-1, CertFail},
	}
	for _, tc := range tests {
		if got := Classify(tc.days); got != tc.want {
			t.Errorf("Classify(%d) = %q, want %q", tc.days, got, tc.want)
		}
	}
}

// testCA is a throwaway CA that signs leaf certificates for the probe tests.
type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Probe Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &testCA{cert: cert, key: key, pool: pool}
}

// leaf issues a certificate for name valid until notAfter.
func (ca *testCA) leaf(t *testing.T, name string, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// serveBySNI starts a TLS server that picks its certificate by SNI.
func serveBySNI(t *testing.T, certs map[string]tls.Certificate) string {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.TLS = &tls.Config{
		GetCertificate: func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
			c, ok := certs[h.ServerName]
			if !ok {
				for _, v := range certs {
					c = v
					break
				}
			}
			return &c, nil
		},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

// TestProbeServed covers SNI selection, leaf fields, and chain verdicts.
func TestProbeServed(t *testing.T) {
	ca := newTestCA(t)
	notAfter := time.Now().Add(40 * 24 * time.Hour).Truncate(time.Second)
	good := ca.leaf(t, "good.test", notAfter)
	other := ca.leaf(t, "other.test", notAfter.Add(24*time.Hour))
	addr := serveBySNI(t, map[string]tls.Certificate{"good.test": good, "other.test": other})
	ctx := context.Background()

	t.Run("sni selects the leaf and fields are reported", func(t *testing.T) {
		got, err := probeServed(ctx, addr, "good.test", 5*time.Second, ca.pool)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(good.Certificate[0])
		if got.SHA256 != hex.EncodeToString(sum[:]) {
			t.Errorf("SHA256 = %s, want %x", got.SHA256, sum)
		}
		if got.IssuerCN != "Probe Test CA" || !got.NotAfter.Equal(notAfter) || got.Host != "good.test" {
			t.Errorf("unexpected cert: %+v", got)
		}
		if len(got.DNSNames) != 1 || got.DNSNames[0] != "good.test" {
			t.Errorf("DNSNames = %v", got.DNSNames)
		}
		if got.ChainErr != "" {
			t.Errorf("ChainErr = %q, want empty for a verifying chain", got.ChainErr)
		}
		o, err := probeServed(ctx, addr, "other.test", 5*time.Second, ca.pool)
		if err != nil || o.SHA256 == got.SHA256 || !o.NotAfter.Equal(notAfter.Add(24*time.Hour)) {
			t.Errorf("other.test must serve a different leaf: %+v err=%v", o, err)
		}
	})

	t.Run("hostname mismatch is ChainErr not error", func(t *testing.T) {
		// good.test's certificate is served for any unknown SNI; verifying it
		// for wrong.test must fail the name check.
		got, err := probeServed(ctx, addr, "wrong.test", 5*time.Second, ca.pool)
		if err != nil {
			t.Fatalf("mismatch must not be an error: %v", err)
		}
		if !strings.Contains(got.ChainErr, "wrong.test") {
			t.Errorf("ChainErr = %q, want hostname mismatch naming wrong.test", got.ChainErr)
		}
		if got.SHA256 == "" {
			t.Error("leaf must still be reported")
		}
	})

	t.Run("untrusted chain against system roots is ChainErr", func(t *testing.T) {
		got, err := ProbeServed(ctx, addr, "good.test", 5*time.Second)
		if err != nil {
			t.Fatalf("untrusted chain must not be an error: %v", err)
		}
		// Wording is platform-specific (Linux: unknown authority; macOS: not trusted).
		if got.ChainErr == "" || strings.Contains(got.ChainErr, "wrong.test") {
			t.Errorf("ChainErr = %q, want a trust failure", got.ChainErr)
		}
	})

	t.Run("httptest self-signed server with SNI", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer srv.Close()
		got, err := ProbeServed(ctx, srv.Listener.Addr().String(), "example.com", 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		want := srv.Certificate()
		sum := sha256.Sum256(want.Raw)
		if got.SHA256 != hex.EncodeToString(sum[:]) || !got.NotAfter.Equal(want.NotAfter) || got.IssuerCN != want.Issuer.CommonName {
			t.Errorf("leaf mismatch: %+v", got)
		}
		if got.ChainErr == "" {
			t.Error("self-signed httptest cert must carry a ChainErr")
		}
	})

	t.Run("empty sni reports the addr host", func(t *testing.T) {
		got, err := probeServed(ctx, addr, "", 5*time.Second, ca.pool)
		if err != nil || got.Host != "127.0.0.1" {
			t.Errorf("Host = %q err=%v", got.Host, err)
		}
	})

	t.Run("dial failure is an error", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		closed := ln.Addr().String()
		_ = ln.Close()
		if _, err := ProbeServed(ctx, closed, "good.test", time.Second); err == nil {
			t.Error("expected a dial error")
		}
	})
}
