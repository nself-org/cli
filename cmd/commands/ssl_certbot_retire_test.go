package commands

// ssl_certbot_retire_test.go: D-0411. Installing the ACME timer or adopting a
// certbot lineage disables nself-ssl-renew.timer, whose deploy hook would copy
// certbot pairs over the ACME generation. systemd is a fake (fakeSystemd); no
// test here or elsewhere in the package reaches the host's systemctl.

import (
	"cmp"
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

// fakeSystemd answers `systemctl show|disable --now|stop` for the certbot
// timer and service from in-memory unit state and records every call.
type fakeSystemd struct {
	load, file, active string // timer state; load "" means systemctl is not installed
	svcActive          string // the service's ActiveState ("" = inactive)
	svcAfter           string // the service's ActiveState once disable/stop ran
	raw                string // when set, `show` of the timer prints this instead
	showErr, disErr    error
	sticky             bool   // disable/stop succeed but change nothing
	onDisable          func() // runs when disable/stop succeeds (a renewal finishing mid-retire)
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
		if args[1] == certbotService {
			return fmt.Sprintf("LoadState=loaded\nActiveState=%s\nUnitFileState=static\n", cmp.Or(s.svcActive, "inactive")), nil
		}
		if s.raw != "" {
			return s.raw, nil
		}
		return fmt.Sprintf("LoadState=%s\nActiveState=%s\nUnitFileState=%s\n", s.load, s.active, s.file), nil
	case "disable", "stop":
		if s.disErr != nil {
			return "Access denied", s.disErr
		}
		if s.svcAfter != "" {
			s.svcActive = s.svcAfter
		}
		if s.onDisable != nil {
			s.onDisable()
		}
		if !s.sticky {
			s.active = "inactive"
			if args[0] == "disable" {
				s.file = "disabled"
			}
		}
		return "", nil
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
	const props = " -p LoadState -p UnitFileState -p ActiveState"
	const svc, show = "show " + certbotService + props, "show " + certbotTimer + props
	const disable, stop = "disable --now " + certbotTimer, "stop " + certbotTimer
	c := func(calls ...string) []string { return append([]string{svc}, calls...) }
	for name, tc := range map[string]struct {
		sd        fakeSystemd
		unitFile  bool
		retired   bool
		wantErr   string
		wantCalls []string
	}{
		"no systemctl":           {wantCalls: c(show)},
		"unknown unit":           {sd: fakeSystemd{load: "not-found", active: "inactive"}, wantCalls: c(show)},
		"already disabled":       {sd: fakeSystemd{load: "loaded", file: "disabled", active: "inactive"}, wantCalls: c(show)},
		"masked":                 {sd: fakeSystemd{load: "masked", file: "masked", active: "inactive"}, wantCalls: c(show)},
		"masked but running":     {sd: fakeSystemd{load: "masked", file: "masked", active: "active"}, retired: true, wantCalls: c(show, stop, show, svc)},
		"enabled and waiting":    {sd: fakeSystemd{load: "loaded", file: "enabled", active: "active"}, retired: true, wantCalls: c(show, disable, show, svc)},
		"enabled, last run fail": {sd: fakeSystemd{load: "loaded", file: "enabled", active: "failed"}, retired: true, wantCalls: c(show, disable, show, svc)},
		"disabled but running":   {sd: fakeSystemd{load: "loaded", file: "disabled", active: "active"}, retired: true, wantCalls: c(show, stop, show, svc)},
		"starting":               {sd: fakeSystemd{load: "loaded", file: "disabled", active: "activating"}, retired: true, wantCalls: c(show, stop, show, svc)},
		"enabled-runtime, idle":  {sd: fakeSystemd{load: "loaded", file: "enabled-runtime", active: "inactive"}, retired: true, wantCalls: c(show, disable, show, svc)},
		"static, idle":           {sd: fakeSystemd{load: "loaded", file: "static", active: "inactive"}, wantCalls: c(show)},
		"linked, idle":           {sd: fakeSystemd{load: "loaded", file: "linked", active: "inactive"}, wantCalls: c(show)},
		"renewal starts during disable": {sd: fakeSystemd{load: "loaded", file: "enabled", active: "active", svcAfter: "active"},
			retired: true, wantErr: "started while the timer was being disabled", wantCalls: c(show, disable, show, svc)},
		"renewal running now": {sd: fakeSystemd{load: "loaded", file: "enabled", active: "active", svcActive: "activating"},
			wantErr: "running a certbot renewal now", wantCalls: c()},
		"disable refused": {sd: fakeSystemd{load: "loaded", file: "enabled", active: "active", disErr: errors.New("exit status 1")},
			wantErr: "Access denied", wantCalls: c(show, disable)},
		"still enabled after disable": {sd: fakeSystemd{load: "loaded", file: "enabled", active: "active", sticky: true},
			wantErr: "still enabled", wantCalls: c(show, disable, show)},
		"no systemctl, unit file present": {unitFile: true, wantErr: "systemctl not found on PATH", wantCalls: c(show)},
		"systemd down, no unit file":      {sd: fakeSystemd{load: "loaded", showErr: errors.New("exit status 1")}, wantCalls: c(show)},
		"systemd down, unit file present": {sd: fakeSystemd{load: "loaded", showErr: errors.New("exit status 1")}, unitFile: true,
			wantErr: "not been booted", wantCalls: c(show)},
		"unexpected output": {sd: fakeSystemd{load: "loaded", raw: "garbage"}, wantErr: "unexpected output", wantCalls: c(show)},
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

// TestCertbotHooks: hook lines in renewal/*.conf and cli.ini and the scripts
// certbot runs from renewal-hooks/{pre,deploy,post} that name an ssl dir are
// found; other keys, comments, scripts certbot skips, other projects and
// longer paths are not.
func TestCertbotHooks(t *testing.T) {
	tmp, _ := filepath.EvalSymlinks(t.TempDir()) // macOS: /var is a symlink
	le, workdir := t.TempDir(), filepath.Join(tmp, "proj")
	sslDir := filepath.Join(workdir, "ssl")
	if hits, err := certbotHooks(filepath.Join(le, "absent"), sslDir); err != nil || len(hits) != 0 {
		t.Fatalf("missing tree: hits=%v err=%v", hits, err)
	}
	file := func(rel, body string, mode os.FileMode) string {
		p := filepath.Join(le, filepath.FromSlash(rel))
		acmeWriteFile(t, p, []byte(body))
		_ = os.Chmod(p, mode)
		return p
	}
	var want []string
	hit := func(p string, line int) { want = append(want, fmt.Sprintf("%s:%d", p, line)) }
	cli := file("cli.ini", "deploy-hook = cp x "+sslDir+"\n; deploy-hook = cp x "+sslDir+"/\npost-hook: cp x "+sslDir+"/\nrenew-hook cp x "+sslDir+"/\nwebroot-path: "+sslDir+"\n", 0o644)
	hit(cli, 1)
	hit(cli, 3)
	hit(cli, 4)
	hit(file("renewal/a.example.org.conf", "version = 2.9.0\n[renewalparams]\n"+nselfDeployHook(workdir)+"post_hook = docker compose exec nginx nginx -s reload\n", 0o644), 3)
	hit(file("renewal/b.example.org.conf", "[renewalparams]\ndeploy_hook = cp x "+filepath.Join(sslDir, "b")+"/\n", 0o644), 2)
	hit(file("renewal/c.example.org.conf", "[renewalparams]\npost_hook = DEST=\""+sslDir+"\"; cp x $DEST\nrenew_hook = cp x /srv/other/ssl/\n# deploy_hook = cp x "+sslDir+"/\nserver = "+sslDir+"\n", 0o644), 2)
	hit(file("renewal/e.example.org.conf", "\"renew_hook\" = cp x "+sslDir+"/\n", 0o644), 1)
	file("renewal/d.example.org.conf.bak", "deploy_hook = cp x "+sslDir+"/\n", 0o644)
	deploy := file("renewal-hooks/deploy/00-copy-cert.sh", "#!/bin/sh\n# copies into "+sslDir+"/ (comment)\ncp \"$RENEWED_LINEAGE\"/*.pem '"+filepath.Join(sslDir, "certificates", "x")+"/'\n", 0o755)
	file("renewal-hooks/deploy/other.sh", "#!/bin/sh\nsystemctl reload postfix\ncp x /data"+sslDir+"/\ncp x "+sslDir+"-old/\nsystemctl reload nginx # was: cp x "+sslDir+"/\n", 0o755)
	quoted := file("renewal-hooks/deploy/20-quoted.sh", "echo \"renewal # done\"; cp x "+sslDir+"/x/ # copy\necho \"$(printf \" # \")\"; cp x "+sslDir+"/\necho $'it\\'s # x'; cp x "+sslDir+"/\n", 0o755)
	redirect := file("renewal-hooks/deploy/10-redirect.sh", "cat \"$RENEWED_LINEAGE/fullchain.pem\" >"+filepath.Join(sslDir, "certificates", "x", "fullchain.pem")+"\n", 0o755)
	if runtime.GOOS != "windows" { // backups certbot ignores are never opened, even a symlink loop
		_ = os.Symlink("loop.conf.bak", filepath.Join(le, "renewal", "loop.conf.bak"))
		_ = os.Symlink("loop.sh~", filepath.Join(le, "renewal-hooks", "deploy", "loop.sh~"))
	}
	file("renewal-hooks/deploy/00-copy-cert.sh~", "cp x "+sslDir+"/\n", 0o755)
	notExec := file("renewal-hooks/deploy/00-copy-cert.sh.off", "cp x "+sslDir+"/\n", 0o644)
	dot := file("renewal-hooks/post/.hidden", "cp x "+sslDir+"/a\n", 0o755)
	file("renewal-hooks/deploy/sub/x", sslDir+"/\n", 0o755)
	hits, err := certbotHooks(le, filepath.Join(t.TempDir(), "served", "ssl"), sslDir)
	hit(deploy, 3)
	if runtime.GOOS == "windows" { // no exec bit: every regular file certbot would see counts
		hit(notExec, 1)
	}
	hit(redirect, 1)
	hit(quoted, 1)
	hit(quoted, 2)
	hit(quoted, 3)
	hit(dot, 1)
	if err != nil || strings.Join(hits, "\n") != strings.Join(want, "\n") {
		t.Fatalf("hits =\n%s\nwant\n%s (err %v)", strings.Join(hits, "\n"), strings.Join(want, "\n"), err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	// A symlinked ssl dir also matches a hook that names its real path.
	link := filepath.Join(t.TempDir(), "ssl")
	_ = os.MkdirAll(sslDir, 0o750)
	if err := os.Symlink(sslDir, link); err == nil {
		if hits, err := certbotHooks(le, link); err != nil || len(hits) != len(want) {
			t.Errorf("symlinked ssl dir: hits=%v err=%v", hits, err)
		}
	}
	// An unreadable hook dir is an error, not "no hooks" (root reads everything).
	if os.Geteuid() != 0 {
		locked := filepath.Join(le, "renewal-hooks", "deploy")
		_ = os.Chmod(locked, 0o000)
		t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })
		if _, err := certbotHooks(le, sslDir); err == nil {
			t.Error("unreadable renewal-hooks/deploy: want an error")
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
	// no systemctl on PATH with the unit file present: refuse, never read it as "no timer"
	sd.load = ""
	if _, err := r.retireCertbot(); err == nil || codeOf(err) != "E151" || !strings.Contains(err.Error(), "with systemctl on PATH") {
		t.Fatalf("no systemctl: err = %v", err)
	}
}
