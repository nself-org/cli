package commands

// ssl_certbot_retire_test.go: D-0411. Installing the ACME timer or adopting a
// certbot lineage disables nself-ssl-renew.timer, whose deploy hook would copy
// certbot pairs over the ACME generation. systemd is a fake (fakeSystemd); no
// test here or elsewhere in the package reaches the host's systemctl.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// realSystemctlRun is the production runner, kept before init swaps it out.
var realSystemctlRun = systemctlRun

// init keeps every test in this package off the host's systemd: by default
// systemctl is absent. Tests that need systemd install a fakeSystemd.
func init() {
	systemctlRun = func(context.Context, ...string) (string, error) { return "", errNoSystemctl }
	certbotDir = filepath.Join(os.TempDir(), "nself-test-no-letsencrypt")
}

// useCertbotDir points certbotDir at a fresh tree for the test.
func useCertbotDir(t *testing.T) string {
	t.Helper()
	old := certbotDir
	certbotDir = t.TempDir()
	t.Cleanup(func() { certbotDir = old })
	return certbotDir
}

// nselfDeployHook is the renewal-conf line certbot saves after a run of the
// nself-ssl-renew service for the project at workdir.
func nselfDeployHook(workdir string) string {
	return "renew_hook = safe=$(basename \"$RENEWED_LINEAGE\" | tr '.:' '--'); dest=" + filepath.Join(workdir, "ssl", "certificates") + "/$safe; mkdir -p \"$dest\"\n"
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
		"starting":               {sd: fakeSystemd{load: "loaded", file: "disabled", active: "activating"}, retired: true, wantCalls: []string{show, disable, show}},
		"enabled-runtime, idle":  {sd: fakeSystemd{load: "loaded", file: "enabled-runtime", active: "inactive"}, retired: true, wantCalls: []string{show, disable, show}},
		"static, idle":           {sd: fakeSystemd{load: "loaded", file: "static", active: "inactive"}, wantCalls: []string{show}},
		"linked, idle":           {sd: fakeSystemd{load: "loaded", file: "linked", active: "inactive"}, wantCalls: []string{show}},
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

// TestCertbotHooks: saved deploy_hook/renew_hook lines and executable deploy
// scripts that write into an ssl dir are found; other keys, comments,
// non-executable scripts, other projects and longer paths are not.
func TestCertbotHooks(t *testing.T) {
	tmp, _ := filepath.EvalSymlinks(t.TempDir()) // macOS: /var is a symlink
	le, workdir := t.TempDir(), filepath.Join(tmp, "proj")
	sslDir := filepath.Join(workdir, "ssl")
	if hits, err := certbotHooks(filepath.Join(le, "absent"), sslDir); err != nil || len(hits) != 0 {
		t.Fatalf("missing tree: hits=%v err=%v", hits, err)
	}
	script := func(name, body string) {
		p := filepath.Join(le, "renewal-hooks", "deploy", name)
		acmeWriteFile(t, p, []byte(body))
		_ = os.Chmod(p, 0o755)
	}
	acmeWriteFile(t, filepath.Join(le, "renewal", "a.example.org.conf"), []byte("version = 2.9.0\n[renewalparams]\n"+nselfDeployHook(workdir)+"post_hook = docker compose exec nginx nginx -s reload\n"))
	acmeWriteFile(t, filepath.Join(le, "renewal", "b.example.org.conf"), []byte("[renewalparams]\ndeploy_hook = cp x "+filepath.Join(sslDir, "b")+"/\n"))
	acmeWriteFile(t, filepath.Join(le, "renewal", "c.example.org.conf"), []byte("[renewalparams]\npost_hook = ls "+sslDir+"/\nrenew_hook = cp x /srv/other/ssl/\n# deploy_hook = cp x "+sslDir+"/\n"))
	script("00-copy-cert.sh", "#!/bin/sh\n# copies into "+sslDir+"/ (comment)\ncp \"$RENEWED_LINEAGE\"/*.pem '"+filepath.Join(sslDir, "certificates", "x")+"/'\n")
	script("other.sh", "#!/bin/sh\nsystemctl reload postfix\ncp x /data"+sslDir+"/\n")
	acmeWriteFile(t, filepath.Join(le, "renewal-hooks", "deploy", "00-copy-cert.sh.bak"), []byte("cp x "+sslDir+"/\n"))
	acmeWriteFile(t, filepath.Join(le, "renewal-hooks", "deploy", "sub", "x"), []byte(sslDir+"/\n"))
	hits, err := certbotHooks(le, filepath.Join(t.TempDir(), "served", "ssl"), sslDir)
	want := []string{
		filepath.Join(le, "renewal", "a.example.org.conf") + ":3",
		filepath.Join(le, "renewal", "b.example.org.conf") + ":2",
		filepath.Join(le, "renewal-hooks", "deploy", "00-copy-cert.sh") + ":3",
	}
	if runtime.GOOS == "windows" { // no exec bit: every regular file in deploy/ counts
		want = append(want, filepath.Join(le, "renewal-hooks", "deploy", "00-copy-cert.sh.bak")+":1")
	}
	if err != nil || strings.Join(hits, "\n") != strings.Join(want, "\n") {
		t.Fatalf("hits =\n%s\nwant\n%s (err %v)", strings.Join(hits, "\n"), strings.Join(want, "\n"), err)
	}

	// A symlinked ssl dir also matches a hook that names its real path.
	if runtime.GOOS != "windows" {
		link := filepath.Join(t.TempDir(), "ssl")
		if err := os.Symlink(sslDir, link); err == nil {
			_ = os.MkdirAll(sslDir, 0o750)
			if hits, err := certbotHooks(le, link); err != nil || len(hits) != len(want) {
				t.Errorf("symlinked ssl dir: hits=%v err=%v", hits, err)
			}
		}
	}
}

// TestRetireCertbotSystemdDownHint: with systemd not answering and the unit file
// present the refusal names the unit files to remove, not a systemctl command.
func TestRetireCertbotSystemdDownHint(t *testing.T) {
	sd := &fakeSystemd{load: "loaded", showErr: errors.New("exit status 1")}
	useSystemd(t, sd)
	acmeWriteFile(t, filepath.Join(systemdUnitDir, certbotTimer), []byte(sslRenewalTimerContent))
	r := &acmeRun{out: io.Discard}
	if _, err := r.retireCertbot(); err == nil || codeOf(err) != "E151" || !strings.Contains(err.Error(), "nself-ssl-renew.{timer,service}") {
		t.Fatalf("err = %v", err)
	}
}
