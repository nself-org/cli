package commands

// ssl_v15_test.go: the v1.5 TLS cut-over (P7-LIVE-22, EPIC D15). In v1.5 mode
// `trust ssl setup|add|renew` run the ACME engine without --acme and failures
// carry E470-E472; in v1.4 mode certbot and E151 are exactly as before. The
// ACME side reuses the prod-layout fixture of ssl_acme_test.go.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
)

// certbotStubs puts logging certbot and docker stubs on PATH and returns the
// log path; any line in it means the real certbot or docker would have run.
func certbotStubs(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub certbot is a POSIX shell script")
	}
	bin, log := t.TempDir(), filepath.Join(t.TempDir(), "calls.log")
	for _, n := range []string{"certbot", "docker"} {
		acmeWriteFile(t, filepath.Join(bin, n), []byte("#!/bin/sh\necho \""+n+" $*\" >> "+log+"\nexit 0\n"))
		_ = os.Chmod(filepath.Join(bin, n), 0o755)
	}
	t.Setenv("PATH", bin)
	return log
}

func readLog(log string) string { b, _ := os.ReadFile(log); return strings.TrimSpace(string(b)) }

// codeOf returns the registry code of err ("" when it carries none).
func codeOf(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// TestSSLV15RoutesACME: with NSELF_V15=1 setup, add and renew without --acme run
// the ACME engine (the stub records the lego RunSpec) and never exec certbot;
// --install-cron selects the nself-acme-renew units.
func TestSSLV15RoutesACME(t *testing.T) {
	log := certbotStubs(t)
	compattest.Set(t, true)
	f := newACMEFix(t)

	out, err := f.run(sslSetupCmd, "--agree-tos")
	if err != nil || len(f.runs) != 1 {
		t.Fatalf("setup without --acme: err=%v lego runs=%d\n%s", err, len(f.runs), out)
	}
	args := strings.Join(f.runs[0].Args, " ")
	if f.runs[0].Image != acme.LegoImage || !strings.Contains(args, "--domains task.nself.org") || !strings.Contains(args, "--dns cloudflare") {
		t.Errorf("RunSpec = %+v", f.runs[0])
	}

	out, err = f.run(sslSetupCmd, "--install-cron", "--dry-run")
	if err != nil || !strings.Contains(out, "nself-acme-renew.timer") || strings.Contains(out, "nself-ssl-renew") {
		t.Errorf("--install-cron must plan the ACME units: err=%v\n%s", err, out)
	}
	if _, err = f.run(sslSetupCmd, "--install-cron"); err != nil || f.timers != 1 {
		t.Errorf("--install-cron: err=%v ACME timers installed=%d (want 1)", err, f.timers)
	}

	before := len(f.runs)
	if _, err = f.run(sslRenewCmd, "--force"); err != nil || len(f.runs) != before+1 {
		t.Errorf("renew without --acme: err=%v lego runs %d -> %d", err, before, len(f.runs))
	}
	if _, err = f.run(sslRenewCmd, "--quiet"); err != nil {
		t.Errorf("an ACME-only flag must be accepted without --acme in v1.5: %v", err)
	}

	addF, _ := addFix(t, nil)
	if _, err = addF.runAdd("app.example.org", "--agree-tos"); err != nil || len(addF.runs) != 1 ||
		!strings.Contains(strings.Join(addF.runs[0].Args, " "), "--http") {
		t.Errorf("add without --acme: err=%v lego runs=%d", err, len(addF.runs))
	}
	if got := readLog(log); got != "" {
		t.Errorf("certbot or docker was executed in v1.5 mode:\n%s", got)
	}
}

// TestSSLV14KeepsCertbot: without NSELF_V15 the same commands exec certbot with
// the argv of origin/main, reload nginx through `docker compose exec`, install
// the certbot cron (setup --install-cron), and never touch the ACME engine.
func TestSSLV14KeepsCertbot(t *testing.T) {
	log := certbotStubs(t)
	compattest.Set(t, false)
	t.Setenv("ENV", "dev")
	runs, crons, timers := 0, 0, 0
	oldCron := installSSLRenewalCron
	installSSLRenewalCron = func(string) error { crons++; return nil }
	t.Cleanup(func() { installSSLRenewalCron = oldCron })
	oldD := acmeD
	acmeD.run = func(context.Context, docker.RunSpec) (string, string, error) { runs++; return "", "", nil }
	acmeD.timer = func(string, string) error { timers++; return nil }
	t.Cleanup(func() {
		acmeD = oldD
		acmeResetFlags(sslSetupCmd)
		acmeResetFlags(sslRenewCmd)
		acmeResetFlags(sslAddCmd)
	})
	live := t.TempDir()
	for _, d := range []string{"example.org", "add.example.org", "renew.invalid"} {
		acmeWriteFile(t, filepath.Join(live, d, "fullchain.pem"), []byte("c"))
		acmeWriteFile(t, filepath.Join(live, d, "privkey.pem"), []byte("k"))
	}
	old := letsEncryptLiveDir
	letsEncryptLiveDir = live
	t.Cleanup(func() { letsEncryptLiveDir = old })
	project := t.TempDir()
	acmeWriteFile(t, filepath.Join(project, ".env"), []byte("BASE_DOMAIN=example.org\nADMIN_EMAIL=ops@example.org\n"))
	t.Chdir(project)

	const dns = "certbot certonly --non-interactive --agree-tos --email ops@example.org --dns-cloudflare --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini"
	const reload = "docker compose exec nginx nginx -s reload"
	for name, tc := range map[string]struct {
		run  func() error
		want string
	}{
		"setup": {func() error { acmeResetFlags(sslSetupCmd); return runSSLSetup(sslSetupCmd, nil) },
			dns + " -d example.org -d api.example.org -d auth.example.org\n" + reload},
		"setup cron": {func() error {
			acmeResetFlags(sslSetupCmd)
			_ = sslSetupCmd.Flags().Set("install-cron", "true")
			return runSSLSetup(sslSetupCmd, nil)
		}, dns + " -d example.org -d api.example.org -d auth.example.org\n" + reload},
		"renew": {func() error { acmeResetFlags(sslRenewCmd); return runSSLRenew(sslRenewCmd, []string{"renew.invalid"}) },
			"certbot renew --cert-name renew.invalid\n" + reload},
		"add": {func() error { acmeResetFlags(sslAddCmd); return runSSLAdd(sslAddCmd, []string{"add.example.org"}) },
			"certbot certonly --non-interactive --agree-tos --email ops@example.org --webroot --webroot-path /var/www/certbot -d add.example.org\ndocker compose exec nginx nginx -t\n" + reload},
	} {
		_ = os.Remove(log)
		if err := tc.run(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := readLog(log); got != tc.want {
			t.Errorf("%s: calls =\n%s\nwant\n%s", name, got, tc.want)
		}
	}
	if runs != 0 {
		t.Errorf("the ACME engine ran %d times in v1.4 mode", runs)
	}
	if timers != 0 {
		t.Errorf("the ACME renewal timer was installed %d times in v1.4 mode", timers)
	}
	if crons != 1 {
		t.Errorf("the certbot renewal cron was installed %d times (want 1, by the setup --install-cron case)", crons)
	}
}

// errCase is one row of TestSSLV15ErrorCodes. drive runs the command under test
// (default: `setup --acme`) after arrange has set up the failure.
type errCase struct {
	name           string
	arrange        func(f *acmeFix)
	drive          func(f *acmeFix) error
	want14, want15 string
}

func driveSetup(f *acmeFix) error { _, err := f.run(sslSetupCmd, "--acme", "--agree-tos"); return err }

// driveAdd runs `ssl add --acme` for a fresh domain on the add fixture.
func driveAdd(f *acmeFix) error {
	_, err := f.runAdd("app.example.org", "--acme", "--agree-tos")
	return err
}

// driveRenew issues the lineages with a working lego, arms the failure, then
// runs `renew --force`.
func driveRenew(fail func(f *acmeFix)) func(f *acmeFix) error {
	return func(f *acmeFix) error {
		if err := driveSetup(f); err != nil {
			return err
		}
		fail(f)
		_, err := f.run(sslRenewCmd, "--acme", "--force")
		return err
	}
}

// TestSSLV15ErrorCodes: an ACME failure is E151 in v1.4 and E470, E471 or E472
// in v1.5 by class; a refusal that never reached the CA stays E151. Each case
// drives the real engine (setup, add and renew), so a reworded engine message
// fails here.
func TestSSLV15ErrorCodes(t *testing.T) {
	legoFails := func(context.Context, docker.RunSpec) (string, string, error) { return "", "boom", errors.New("exit 1") }
	cases := []errCase{
		{"lego fails", func(f *acmeFix) { acmeD.run = legoFails }, driveSetup, "E151", "E470"},
		{"nginx -t fails after issue", func(f *acmeFix) {
			acmeD.exec = func(context.Context, string, []string) (string, string, error) {
				return "", "bad conf", errors.New("exit 1")
			}
		}, driveSetup, "E151", "E471"},
		{"dns credential missing", func(f *acmeFix) { delete(f.secrets, "SSL_DNS_CLOUDFLARE_API_TOKEN") }, driveSetup, "E151", "E472"},
		{"refusal before the CA", func(f *acmeFix) {
			acmeWriteFile(t, filepath.Join(f.project, ".env"), []byte("BASE_DOMAIN=localhost\nENV=dev\nPROJECT_NAME=backend\nNGINX_FRONTED_BY=nself-web\n"))
		}, driveSetup, "E151", "E151"},
		{"add: served certificate check fails", func(f *acmeFix) {
			acmeD.probe = func(context.Context, string, string, time.Duration) (ssl.ServedCert, error) {
				return ssl.ServedCert{}, errors.New("dial refused")
			}
		}, driveAdd, "E151", "E471"},
		{"add: lego fails", func(f *acmeFix) { acmeD.run = legoFails }, driveAdd, "E151", "E470"},
		{"renew: lego fails", func(f *acmeFix) {}, driveRenew(func(f *acmeFix) { acmeD.run = legoFails }), "E151", "E470"},
	}
	compattest.Both(t, func(t *testing.T) {
		for _, c := range cases {
			var f *acmeFix
			if strings.HasPrefix(c.name, "add:") {
				f, _ = addFix(t, nil)
			} else {
				f = newACMEFix(t)
			}
			c.arrange(f)
			want := c.want14
			if strings.HasSuffix(t.Name(), "v1.5") {
				want = c.want15
			}
			err := c.drive(f)
			if err == nil {
				t.Fatalf("%s: no error", c.name)
			}
			if got := codeOf(err); got != want {
				t.Errorf("%s: code = %q, want %q (%v)", c.name, got, want, err)
			}
			if want != "E151" {
				var ce *errs.CLIError
				if errors.As(err, &ce) && (ce.Fix == "" || ce.Wrapped == nil) {
					t.Errorf("%s: re-coded error lost its fix or cause: %+v", c.name, ce)
				}
			}
		}
	})
}

// TestSSLV15AddInstallKeepsMessage: tagging an after-install failure for E471
// must not change the message or lose the *acme.Error (v1.4 text is frozen).
func TestSSLV15AddInstallKeepsMessage(t *testing.T) {
	base := acme.Refuse("fix it", "served certificate check failed: %v", "dial refused")
	tagged := installFailed(base)
	if tagged.Error() != base.Error() {
		t.Errorf("message changed: %q -> %q", base.Error(), tagged.Error())
	}
	var ae *acme.Error
	if !errors.As(tagged, &ae) || !errors.Is(tagged, acme.ErrInstall) {
		t.Errorf("chain lost the *acme.Error or ErrInstall: %v", tagged)
	}
	if got := acme.CodeFor(tagged); got != acme.CodeInstall {
		t.Errorf("CodeFor(tagged) = %q, want %q", got, acme.CodeInstall)
	}
	if got := acme.CodeFor(base); got != "" {
		t.Errorf("CodeFor(untagged refusal) = %q, want none", got)
	}
}

// TestSSLV15CodeForUnclassified: errors that are none of the three classes, and
// nil, map to no code, so they keep E151.
func TestSSLV15CodeForUnclassified(t *testing.T) {
	for _, err := range []error{nil, errors.New("writing lineages.json: disk full"), acme.Refuse("fix", "BASE_DOMAIN must be a real domain")} {
		if got := acme.CodeFor(err); got != "" {
			t.Errorf("CodeFor(%v) = %q, want none", err, got)
		}
	}
}
