package ssl

// generator_test.go — tests for the skip predicates ssl.Generator and the
// build plan mode share (P7-LIVE-21 review S4).
// Inputs: temp dirs with self-signed certificates. Outputs: pass/fail.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNeedsCerts(t *testing.T) {
	cases := map[string]bool{
		"":            true,
		"local":       true,
		"mkcert":      true,
		"letsencrypt": false,
		"custom":      false,
		"none":        false,
	}
	for mode, want := range cases {
		if got := NeedsCerts(mode); got != want {
			t.Errorf("NeedsCerts(%q) = %v, want %v", mode, got, want)
		}
	}
}

func writePair(t *testing.T, dir string, notAfter time.Time) {
	t.Helper()
	src := writeSelfSignedCert(t, time.Now().Add(-time.Hour), notAfter)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCertPairValid(t *testing.T) {
	t.Run("fresh pair", func(t *testing.T) {
		dir := t.TempDir()
		writePair(t, dir, time.Now().Add(90*24*time.Hour))
		if !CertPairValid(dir) {
			t.Error("a 90-day pair must be valid")
		}
	})
	t.Run("near expiry", func(t *testing.T) {
		dir := t.TempDir()
		writePair(t, dir, time.Now().Add(10*24*time.Hour))
		if CertPairValid(dir) {
			t.Error("a 10-day pair must be regenerated")
		}
	})
	t.Run("expired", func(t *testing.T) {
		dir := t.TempDir()
		writePair(t, dir, time.Now().Add(-24*time.Hour))
		if CertPairValid(dir) {
			t.Error("an expired pair must not be valid")
		}
	})
	t.Run("missing private key", func(t *testing.T) {
		dir := t.TempDir()
		writePair(t, dir, time.Now().Add(90*24*time.Hour))
		if err := os.Remove(filepath.Join(dir, "privkey.pem")); err != nil {
			t.Fatal(err)
		}
		if CertPairValid(dir) {
			t.Error("a pair without privkey.pem must not be valid")
		}
	})
	t.Run("garbage chain", func(t *testing.T) {
		dir := t.TempDir()
		for _, f := range []string{"fullchain.pem", "privkey.pem"} {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("not pem"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if CertPairValid(dir) {
			t.Error("an unparsable chain must not be valid")
		}
	})
	t.Run("empty dir", func(t *testing.T) {
		if CertPairValid(t.TempDir()) {
			t.Error("an empty dir must not be valid")
		}
	})
}
