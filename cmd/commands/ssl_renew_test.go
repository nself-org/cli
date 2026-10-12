package commands

// ssl_renew_test.go — Regression coverage for the T15 fix: `nself ssl renew`
// used to never call installIssuedCert, so a genuinely-renewed Let's Encrypt
// cert could sit on disk unused while nginx kept serving the stale one.
//
// installRenewedCertIfDue is the extracted, testable half of runSSLRenew
// (the certbot exec itself is not testable without a certbot binary): given
// a "before" mtime captured prior to renewal, it decides whether certbot
// actually renewed the lineage and, if so, installs it exactly where
// ssl_add.go/ssl_setup.go already do.
//
// Inputs: a domain, a scratch workdir, and a beforeMtime.
// Outputs: assertions on whether ssl/certificates/<domain-safe>/ was
// populated (and at 0600) or left untouched.
// Constraints: no certbot, docker, or network.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestInstallRenewedCertIfDue_InstallsWhenRenewed proves the T15 fix: when
// the live cert's mtime moved past the pre-renewal baseline (certbot actually
// renewed it), the renewed files land in ssl/certificates/<domain-safe>/ at
// 0600 — the path nginx reads — closing the gap where a renewed cert never
// reached nginx.
//
// Before the fix, runSSLRenew never called installIssuedCert at all, so this
// exact scenario left ssl/certificates/ empty; this test fails against that
// code (no such function existed) and passes once installRenewedCertIfDue is
// wired in and called after a genuine renewal.
func TestInstallRenewedCertIfDue_InstallsWhenRenewed(t *testing.T) {
	const domain = "renew.nself.org"

	liveRoot := t.TempDir()
	liveDomainDir := filepath.Join(liveRoot, domain)
	if err := os.MkdirAll(liveDomainDir, 0o750); err != nil {
		t.Fatalf("mkdir live domain dir: %v", err)
	}

	orig := letsEncryptLiveDir
	letsEncryptLiveDir = liveRoot
	t.Cleanup(func() { letsEncryptLiveDir = orig })

	// beforeMtime simulates "captured just before certbot ran".
	beforeMtime := time.Now().Add(-1 * time.Hour)

	// certbot "renewed" the lineage: fullchain/privkey now exist with a
	// current mtime, which is after beforeMtime.
	for name, body := range map[string]string{
		"fullchain.pem": "RENEWED-FULLCHAIN",
		"privkey.pem":   "RENEWED-PRIVKEY",
	} {
		if err := os.WriteFile(filepath.Join(liveDomainDir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	workdir := t.TempDir()
	renewed, err := installRenewedCertIfDue(domain, workdir, beforeMtime)
	if err != nil {
		t.Fatalf("installRenewedCertIfDue: %v", err)
	}
	if !renewed {
		t.Fatal("expected renewed=true when the live cert's mtime is after beforeMtime")
	}

	certDir := filepath.Join(workdir, "ssl", "certificates", domainToFilesafe(domain))
	for name, want := range map[string]string{
		"fullchain.pem": "RENEWED-FULLCHAIN",
		"privkey.pem":   "RENEWED-PRIVKEY",
	} {
		path := filepath.Join(certDir, name)
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("expected %s to exist: %v", path, statErr)
		}
		// Windows does not implement Unix permission bits — Go reports 0666
		// there regardless of what os.WriteFile was given, so this assertion
		// can only be made where the permission actually exists. Same guard
		// as internal/sentryapi/client_test.go and the sibling ssl tests.
		if runtime.GOOS != "windows" {
			if perm := info.Mode().Perm(); perm != 0o600 {
				t.Errorf("%s: got perm %o, want 0600", path, perm)
			}
		}
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}

// TestInstallRenewedCertIfDue_SkipsWhenNotDue proves the "not unconditional"
// half of the fix: when certbot's renew was a no-op (the lineage is not yet
// within its renewal window, so the live cert's mtime did NOT move past
// beforeMtime), installRenewedCertIfDue must not touch the filesystem at
// all — no ssl/certificates/ dir is created, and installIssuedCert is not
// invoked. An unconditional install would pass the happy-path test above but
// would needlessly rewrite unchanged files on every `ssl renew` call, and
// could mask a certbot failure by reinstalling stale local files as fresh.
func TestInstallRenewedCertIfDue_SkipsWhenNotDue(t *testing.T) {
	const domain = "notdue.nself.org"

	liveRoot := t.TempDir()
	liveDomainDir := filepath.Join(liveRoot, domain)
	if err := os.MkdirAll(liveDomainDir, 0o750); err != nil {
		t.Fatalf("mkdir live domain dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveDomainDir, "fullchain.pem"), []byte("OLD-FULLCHAIN"), 0o600); err != nil {
		t.Fatalf("seed fullchain.pem: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveDomainDir, "privkey.pem"), []byte("OLD-PRIVKEY"), 0o600); err != nil {
		t.Fatalf("seed privkey.pem: %v", err)
	}

	orig := letsEncryptLiveDir
	letsEncryptLiveDir = liveRoot
	t.Cleanup(func() { letsEncryptLiveDir = orig })

	// beforeMtime is in the future relative to the file we just wrote, so
	// the file's mtime cannot be "after" it — simulating a certbot renew
	// that was a no-op because the lineage isn't due yet.
	beforeMtime := time.Now().Add(1 * time.Hour)

	workdir := t.TempDir()
	renewed, err := installRenewedCertIfDue(domain, workdir, beforeMtime)
	if err != nil {
		t.Fatalf("installRenewedCertIfDue: %v", err)
	}
	if renewed {
		t.Fatal("expected renewed=false when the live cert's mtime did not move past beforeMtime")
	}

	certDir := filepath.Join(workdir, "ssl", "certificates", domainToFilesafe(domain))
	if _, statErr := os.Stat(certDir); !os.IsNotExist(statErr) {
		t.Errorf("expected %s to not be created on a not-yet-due renewal, stat err: %v", certDir, statErr)
	}
}

// TestCertbotRenewRefusesUnderACME: the v1.4 certbot `ssl renew <domain>`
// refuses before running certbot in a project whose certificates ACME
// manages, or whose lineages.json cannot be read (D-0461); without lineages
// it still reaches certbot (positive control), and without a domain it only
// reloads nginx.
func TestCertbotRenewRefusesUnderACME(t *testing.T) {
	log := certbotStubs(t)
	compattest.Set(t, false)
	t.Setenv("ENV", "dev")
	live, oldLive := t.TempDir(), letsEncryptLiveDir
	letsEncryptLiveDir = live
	t.Cleanup(func() { letsEncryptLiveDir = oldLive; acmeResetFlags(sslRenewCmd) })
	project := t.TempDir()
	acmeWriteFile(t, filepath.Join(project, ".env"), []byte("BASE_DOMAIN=localhost\n"))
	t.Chdir(project)
	state := filepath.Join(project, "ssl", ".acme", "lineages.json")
	const lineage = `{"schema_version":"1","lineages":[{"name":"localhost","targets":["certificates/localhost"]}]}`

	for _, tc := range []struct {
		name, lineages, wantErr string
		args                    []string
		certbotRuns             bool
	}{
		// certbot runs; the stub renews nothing, so the install step reports it.
		{name: "no ACME state", args: []string{"localhost"}, wantErr: "did certbot succeed", certbotRuns: true},
		{name: "ACME lineages", lineages: lineage, args: []string{"localhost"}, wantErr: "nself trust ssl renew --acme"},
		{name: "unreadable lineages.json", lineages: "{", args: []string{"localhost"}, wantErr: "reading the ACME lineages"},
		{name: "reload only under ACME", lineages: lineage},
	} {
		_ = os.Remove(log)
		_ = os.Remove(state)
		if tc.lineages != "" {
			acmeWriteFile(t, state, []byte(tc.lineages))
		}
		acmeResetFlags(sslRenewCmd)
		err := runSSLRenew(sslRenewCmd, tc.args)
		if (err == nil) != (tc.wantErr == "") || (err != nil && !strings.Contains(err.Error(), tc.wantErr)) {
			t.Fatalf("%s: err=%v, want %q", tc.name, err, tc.wantErr)
		}
		if got := strings.Contains(readLog(log), "certbot renew"); got != tc.certbotRuns {
			t.Errorf("%s: certbot ran=%v (want %v), calls=%q", tc.name, got, tc.certbotRuns, readLog(log))
		}
	}
	if _, err := os.Stat(filepath.Join(project, "ssl", "certificates")); !os.IsNotExist(err) {
		t.Errorf("ssl/certificates was written: %v (want nothing installed)", err)
	}
}
