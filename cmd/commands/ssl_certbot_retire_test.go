package commands

// ssl_certbot_retire_test.go: D-0411. Installing the ACME timer or adopting a
// certbot lineage disables nself-ssl-renew.timer, whose deploy hook would copy
// certbot pairs over the ACME generation. systemd is a fake (fakeSystemd); no
// test here or elsewhere in the package reaches the host's systemctl.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/ssl/acme"
)

// realSystemctlRun is the production runner, kept before init swaps it out.
var realSystemctlRun = systemctlRun

// init keeps every test in this package off the host's systemd: by default
// systemctl is absent. Tests that need systemd install a fakeSystemd.
func init() {
	systemctlRun = func(context.Context, ...string) (string, error) { return "", errNoSystemctl }
}

// fakeSystemd answers `systemctl show` and `systemctl disable --now` for the
// certbot timer from in-memory unit state and records every call.
type fakeSystemd struct {
	load, file, active string // unit state; load "" means systemctl is not installed
	raw                string // when set, `show` prints this instead
	showErr, disErr    error
	sticky             bool // disable succeeds but leaves the unit enabled
	calls              []string
}

func (s *fakeSystemd) run(_ context.Context, args ...string) (string, error) {
	s.calls = append(s.calls, strings.Join(args, " "))
	if s.load == "" {
		return "", errNoSystemctl
	}
	switch args[0] {
	case "show":
		if s.showErr != nil {
			return "System has not been booted with systemd", s.showErr
		}
		if s.raw != "" {
			return s.raw, nil
		}
		return fmt.Sprintf("LoadState=%s\nActiveState=%s\nUnitFileState=%s\n", s.load, s.active, s.file), nil
	case "disable":
		if s.disErr != nil {
			return "Access denied", s.disErr
		}
		if !s.sticky {
			s.file, s.active = "disabled", "inactive"
		}
		return "Removed timers.target.wants/" + certbotTimer, nil
	}
	return "", fmt.Errorf("unexpected systemctl %v", args)
}

// disables counts the `systemctl disable` calls.
func (s *fakeSystemd) disables() (n int) {
	for _, c := range s.calls {
		if strings.HasPrefix(c, "disable") {
			n++
		}
	}
	return n
}

// useSystemd installs s as the systemctl runner and an empty unit dir.
func useSystemd(t *testing.T, s *fakeSystemd) {
	t.Helper()
	oldRun, oldDir := systemctlRun, systemdUnitDir
	systemctlRun, systemdUnitDir = s.run, t.TempDir()
	t.Cleanup(func() { systemctlRun, systemdUnitDir = oldRun, oldDir })
}

func TestRetireCertbotTimer(t *testing.T) {
	const show = "show " + certbotTimer + " -p LoadState -p UnitFileState -p ActiveState"
	const disable = "disable --now " + certbotTimer
	for name, tc := range map[string]struct {
		sd        fakeSystemd
		unitFile  bool
		retired   bool
		wantErr   string
		wantCalls []string
	}{
		"no systemctl":           {wantCalls: []string{show}},
		"unknown unit":           {sd: fakeSystemd{load: "not-found", active: "inactive"}, wantCalls: []string{show}},
		"already disabled":       {sd: fakeSystemd{load: "loaded", file: "disabled", active: "inactive"}, wantCalls: []string{show}},
		"masked":                 {sd: fakeSystemd{load: "masked", file: "masked", active: "inactive"}, wantCalls: []string{show}},
		"enabled and waiting":    {sd: fakeSystemd{load: "loaded", file: "enabled", active: "active"}, retired: true, wantCalls: []string{show, disable, show}},
		"enabled, last run fail": {sd: fakeSystemd{load: "loaded", file: "enabled", active: "failed"}, retired: true, wantCalls: []string{show, disable, show}},
		"disabled but running":   {sd: fakeSystemd{load: "loaded", file: "disabled", active: "active"}, retired: true, wantCalls: []string{show, disable, show}},
		"disable refused": {sd: fakeSystemd{load: "loaded", file: "enabled", active: "active", disErr: errors.New("exit status 1")},
			wantErr: "Access denied", wantCalls: []string{show, disable}},
		"still enabled after disable": {sd: fakeSystemd{load: "loaded", file: "enabled", active: "active", sticky: true},
			wantErr: "still enabled", wantCalls: []string{show, disable, show}},
		"systemd down, no unit file": {sd: fakeSystemd{load: "loaded", showErr: errors.New("exit status 1")}, wantCalls: []string{show}},
		"systemd down, unit file present": {sd: fakeSystemd{load: "loaded", showErr: errors.New("exit status 1")}, unitFile: true,
			wantErr: "not been booted", wantCalls: []string{show}},
		"unexpected output": {sd: fakeSystemd{load: "loaded", raw: "garbage"}, wantErr: "unexpected output", wantCalls: []string{show}},
	} {
		t.Run(name, func(t *testing.T) {
			sd := tc.sd
			useSystemd(t, &sd)
			if tc.unitFile {
				acmeWriteFile(t, filepath.Join(systemdUnitDir, certbotTimer), []byte(sslRenewalTimerContent))
			}
			retired, err := retireCertbotTimer(context.Background())
			if retired != tc.retired || (err == nil) != (tc.wantErr == "") || (err != nil && !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("retired=%v err=%v, want retired=%v err~%q", retired, err, tc.retired, tc.wantErr)
			}
			if strings.Join(sd.calls, "\n") != strings.Join(tc.wantCalls, "\n") {
				t.Errorf("calls =\n%s\nwant\n%s", strings.Join(sd.calls, "\n"), strings.Join(tc.wantCalls, "\n"))
			}
		})
	}
}

// TestRetireCertbotTimerIdempotent: a second run reads the state and changes nothing.
func TestRetireCertbotTimerIdempotent(t *testing.T) {
	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active"}
	useSystemd(t, sd)
	for i, want := range []bool{true, false, false} {
		if retired, err := retireCertbotTimer(context.Background()); err != nil || retired != want {
			t.Fatalf("run %d: retired=%v err=%v, want %v", i, retired, err, want)
		}
	}
	if sd.disables() != 1 {
		t.Errorf("disable ran %d times, want 1\n%s", sd.disables(), strings.Join(sd.calls, "\n"))
	}
}

// TestSystemctlRunReal: the production runner reports a missing systemctl as
// errNoSystemctl and otherwise returns the binary's output (a stub on PATH).
func TestSystemctlRunReal(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := realSystemctlRun(context.Background(), "show", certbotTimer); !errors.Is(err, errNoSystemctl) {
		t.Fatalf("no systemctl on PATH: err=%v, want errNoSystemctl", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("stub systemctl is a POSIX shell script")
	}
	bin := t.TempDir()
	acmeWriteFile(t, filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\necho \"LoadState=not-found $*\"\n"))
	_ = os.Chmod(filepath.Join(bin, "systemctl"), 0o755)
	t.Setenv("PATH", bin)
	if out, err := realSystemctlRun(context.Background(), "show", certbotTimer); err != nil || strings.TrimSpace(out) != "LoadState=not-found show "+certbotTimer {
		t.Fatalf("stub systemctl: out=%q err=%v", out, err)
	}
}

// TestACMETimerRetiresCertbotTimer: `setup --acme --install-cron` disables the
// certbot timer before installing the ACME one; a re-run changes nothing; a
// refused disable installs nothing and names the manual command; --dry-run only reports.
func TestACMETimerRetiresCertbotTimer(t *testing.T) {
	f := newACMEFix(t)
	if err := acme.Save(f.ssl, acme.File{Contact: "ops@example.org", Lineages: []acme.Lineage{{Name: "x", Challenge: "dns-01", DNSProvider: "cloudflare", Targets: []string{"certificates/x"}}}}); err != nil {
		t.Fatal(err)
	}

	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active", disErr: errors.New("exit status 1")}
	useSystemd(t, sd)
	_, err := f.run(sslSetupCmd, "--acme", "--install-cron")
	if err == nil || codeOf(err) != "E151" || !strings.Contains(err.Error(), "systemctl disable --now "+certbotTimer) || f.timers != 0 {
		t.Fatalf("refused disable: err=%v timers=%d (want E151 naming the command, no ACME timer)", err, f.timers)
	}

	sd.disErr, sd.calls = nil, nil
	out, err := f.run(sslSetupCmd, "--acme", "--install-cron", "--dry-run")
	if err != nil || !strings.Contains(out, "would disable "+certbotTimer) || sd.disables() != 0 || f.timers != 0 {
		t.Fatalf("dry run: err=%v disables=%d timers=%d\n%s", err, sd.disables(), f.timers, out)
	}

	out, err = f.run(sslSetupCmd, "--acme", "--install-cron")
	if err != nil || !strings.Contains(out, "disabled "+certbotTimer) || sd.disables() != 1 || f.timers != 1 || sd.file != "disabled" {
		t.Fatalf("install: err=%v disables=%d timers=%d file=%s\n%s", err, sd.disables(), f.timers, sd.file, out)
	}
	if out, err = f.run(sslSetupCmd, "--acme", "--install-cron"); err != nil || strings.Contains(out, "disabled "+certbotTimer) || sd.disables() != 1 {
		t.Fatalf("re-run must not disable again: err=%v disables=%d\n%s", err, sd.disables(), out)
	}
}

// TestACMEAdoptRetiresCertbotTimer: adoption disables the certbot timer after
// the lineages are saved; a refused disable keeps them (a re-run finishes) and
// names the manual command; --dry-run only reports.
func TestACMEAdoptRetiresCertbotTimer(t *testing.T) {
	f := newACMEFix(t)
	le := adoptFix(t, f)
	cred := filepath.Join(f.root, "cloudflare.ini")
	acmeWriteFile(t, cred, []byte("# certbot\ndns_cloudflare_api_token = fake-token-not-a-secret\n"))
	adopt := []string{"--acme", "--adopt-certbot=" + le, "--dns-credential-file=" + cred}

	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active"}
	useSystemd(t, sd)
	out, err := f.run(sslSetupCmd, append(adopt, "--dry-run")...)
	if err != nil || !strings.Contains(out, "would disable "+certbotTimer) || sd.disables() != 0 {
		t.Fatalf("dry run: err=%v disables=%d\n%s", err, sd.disables(), out)
	}

	sd.disErr = errors.New("exit status 1")
	_, err = f.run(sslSetupCmd, adopt...)
	if lf, _ := acme.Load(f.ssl); err == nil || !strings.Contains(err.Error(), "systemctl disable --now "+certbotTimer) || len(lf.Lineages) != 2 {
		t.Fatalf("refused disable: err=%v lineages=%d (want the fix hint and both lineages kept)", err, len(lf.Lineages))
	}

	sd.disErr = nil
	if out, err = f.run(sslSetupCmd, adopt...); err != nil || !strings.Contains(out, "disabled "+certbotTimer) || sd.file != "disabled" {
		t.Fatalf("adopt: err=%v file=%s\n%s", err, sd.file, out)
	}
	if lf, _ := acme.Load(f.ssl); len(lf.Lineages) != 2 {
		t.Errorf("lineages = %+v", lf.Lineages)
	}
}

// TestCertbotSetupSkipsTimerUnderACME: the v1.4 certbot `setup --install-cron`
// does not reinstall the certbot timer in a project whose certificates ACME manages.
func TestCertbotSetupSkipsTimerUnderACME(t *testing.T) {
	certbotStubs(t)
	compattest.Set(t, false)
	t.Setenv("ENV", "dev")
	crons := 0
	oldCron := installSSLRenewalCron
	installSSLRenewalCron = func(string) error { crons++; return nil }
	live, oldLive := t.TempDir(), letsEncryptLiveDir
	letsEncryptLiveDir = live
	t.Cleanup(func() {
		installSSLRenewalCron, letsEncryptLiveDir = oldCron, oldLive
		acmeResetFlags(sslSetupCmd)
	})
	acmeWriteFile(t, filepath.Join(live, "example.org", "fullchain.pem"), []byte("c"))
	acmeWriteFile(t, filepath.Join(live, "example.org", "privkey.pem"), []byte("k"))
	project := t.TempDir()
	acmeWriteFile(t, filepath.Join(project, ".env"), []byte("BASE_DOMAIN=example.org\nADMIN_EMAIL=ops@example.org\n"))
	t.Chdir(project)

	for _, tc := range []struct {
		acme bool
		want int
	}{{false, 1}, {true, 1}} {
		if tc.acme {
			if err := acme.Save(filepath.Join(project, "ssl"), acme.File{Lineages: []acme.Lineage{{Name: "example-org", Targets: []string{"certificates/example-org"}}}}); err != nil {
				t.Fatal(err)
			}
		}
		acmeResetFlags(sslSetupCmd)
		_ = sslSetupCmd.Flags().Set("install-cron", "true")
		if err := runSSLSetup(sslSetupCmd, nil); err != nil {
			t.Fatalf("acme=%v: %v", tc.acme, err)
		}
		if crons != tc.want {
			t.Errorf("acme=%v: certbot timer installs = %d, want %d", tc.acme, crons, tc.want)
		}
	}
}
