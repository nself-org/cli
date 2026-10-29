package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetSnapshotFrontSSL(t *testing.T) {
	f := newFakeRemote(t)
	front := t.TempDir()
	certs := filepath.Join(front, "ssl", "certificates")
	for _, n := range []string{"fullchain.pem", "privkey.pem", "chain.pem"} {
		writeFile(t, filepath.Join(certs, "api-example-com", n), "KEYMATERIAL-"+n, 0o600)
	}
	writeFile(t, filepath.Join(certs, "only-cert-example-org", "fullchain.pem"), "CERT", 0o644)
	writeFile(t, filepath.Join(certs, ".api-example-com.gen-1", "fullchain.pem"), "OLD", 0o644) // a generation dir is not a cert dir
	got, err := TakeFrontSSL(context.Background(), f, front)
	if err != nil {
		t.Fatal(err)
	}
	want := []CertDir{{Name: "api-example-com", Fullchain: true, Privkey: true, Chain: true}, {Name: "only-cert-example-org", Fullchain: true}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("front ssl = %+v, want %+v", got, want)
	}
	for _, c := range f.commands() {
		if strings.Contains(c, "cat ") || strings.Contains(c, "head ") || strings.Contains(c, "grep ") {
			t.Errorf("the ssl read opened a file: %q", c)
		}
	}
	// A front root with no ssl dir has no certificates.
	none, err := TakeFrontSSL(context.Background(), f, t.TempDir())
	if err != nil || len(none) != 0 {
		t.Errorf("empty front: %v %v", none, err)
	}
	// Drift: a cert dir gone or changed since the snapshot the build used.
	if d := frontSSLDrift(want, got); len(d) != 0 {
		t.Errorf("no drift expected: %v", d)
	}
	d := frontSSLDrift(want, want[:1])
	if len(d) != 1 || !strings.Contains(d[0], "fronting stack: ssl dir \"only-cert-example-org\" is missing") {
		t.Errorf("drift = %v", d)
	}
	// A host name that is not a plain name is refused, never used as a local path.
	writeFile(t, filepath.Join(certs, "bad name", "fullchain.pem"), "x", 0o644)
	if _, err := TakeFrontSSL(context.Background(), f, front); !e485(err) {
		t.Errorf("unsafe cert dir name: %v", err)
	}
}

// D-0824: only a missing certificates dir is empty; a find that cannot read one
// is an error, not "no certificates".
func TestTargetSnapshotFrontSSLFindErrorIsNotEmpty(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every directory")
	}
	f := newFakeRemote(t)
	front := t.TempDir()
	certs := filepath.Join(front, "ssl", "certificates")
	writeFile(t, filepath.Join(certs, "api-example-com", "fullchain.pem"), "CERT", 0o644)
	if err := os.Chmod(certs, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(certs, 0o755) })
	if got, err := TakeFrontSSL(context.Background(), f, front); err == nil {
		t.Errorf("an unreadable certificates dir listed as %+v with no error", got)
	}
}
