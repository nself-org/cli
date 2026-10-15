package commands

// ssl_certbot_retire_flow_test.go: D-0411 through the commands. `setup --acme`
// (issue, --install-cron, --adopt-certbot) refuses while a certbot hook writes
// into the ssl dir and disables nself-ssl-renew.timer; the v1.4 certbot
// `ssl setup` refuses in a project whose certificates ACME manages. systemd is
// the fakeSystemd of ssl_certbot_retire_test.go.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/ssl/acme"
)

// TestACMETimerRechecksHooksAfterEarlierRetirement covers D-0597: a hook
// appearing between timerAfter's scans still reports an earlier retirement.
func TestACMETimerRechecksHooksAfterEarlierRetirement(t *testing.T) {
	f := newACMEFix(t)
	le := useCertbotDir(t)
	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active"}
	useSystemd(t, sd)
	var out bytes.Buffer
	r := &acmeRun{out: &out, workdir: f.project, res: &acme.Resolution{SSLDir: f.ssl}}
	retired, err := r.retireCertbot()
	if err != nil || !retired || sd.disables() != 1 {
		t.Fatalf("earlier retirement: retired=%v err=%v disables=%d", retired, err, sd.disables())
	}
	out.Reset()
	hook := filepath.Join(le, "renewal", "x.conf")
	scans := 0
	acmeD.certbotHooks = func(dir string, sslDirs ...string) ([]string, error) {
		scans++
		if scans == 2 {
			acmeWriteFile(t, hook, []byte("[renewalparams]\n"+nselfDeployHook(f.project)))
		}
		return certbotHooks(dir, sslDirs...)
	}
	err = r.timerAfter(retired)
	if err == nil || codeOf(err) != "E151" || !strings.Contains(err.Error(), hook+":2") {
		t.Fatalf("hook saved between scans: err=%v (want E151 naming %s:2)", err, hook)
	}
	if scans != 2 || sd.disables() != 1 || f.timers != 0 || strings.Count(out.String(), "was disabled before this failure") != 1 {
		t.Fatalf("scans=%d disables=%d timers=%d\n%s", scans, sd.disables(), f.timers, out.String())
	}
}

// TestACMETimerRetiresCertbotTimer: `setup --acme --install-cron` disables the
// certbot timer before installing the ACME one; a re-run changes nothing; a
// refused disable or a saved certbot hook installs nothing and names the fix;
// --dry-run only reports.
func TestACMETimerRetiresCertbotTimer(t *testing.T) {
	f := newACMEFix(t)
	le := useCertbotDir(t)
	if err := acme.Save(f.ssl, acme.File{Contact: "ops@example.org", Lineages: []acme.Lineage{{Name: "x", Challenge: "dns-01", DNSProvider: "cloudflare", Targets: []string{"certificates/x"}}}}); err != nil {
		t.Fatal(err)
	}
	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active"}
	useSystemd(t, sd)

	hook := filepath.Join(le, "renewal", "x.conf")
	acmeWriteFile(t, hook, []byte("[renewalparams]\n"+nselfDeployHook(f.project)))
	for _, dry := range []string{"--dry-run=false", "--dry-run"} {
		_, err := f.run(sslSetupCmd, "--acme", "--install-cron", dry)
		if err == nil || codeOf(err) != "E151" || !strings.Contains(err.Error(), hook+":2") || sd.disables() != 0 || f.timers != 0 {
			t.Fatalf("saved hook %s: err=%v disables=%d timers=%d (want E151 naming %s:2, nothing changed)", dry, err, sd.disables(), f.timers, hook)
		}
	}
	_ = os.Remove(hook)

	sd.disErr = errors.New("exit status 1")
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

// TestACMEAdoptRetiresCertbotTimer: adoption refuses, writing nothing, while a
// certbot deploy script copies into the served ssl dir (outside the project, as
// with NGINX_FRONTED_BY on prod); otherwise it disables the certbot
// timer before its first write and says no renewal is scheduled, or with
// --install-cron installs the ACME timer. A refused disable writes nothing.
func TestACMEAdoptRetiresCertbotTimer(t *testing.T) {
	f := newACMEFix(t)
	le := adoptFix(t, f)
	cred := filepath.Join(f.root, "cloudflare.ini")
	acmeWriteFile(t, cred, []byte("# certbot\ndns_cloudflare_api_token = fake-token-not-a-secret\n"))
	adopt := []string{"--acme", "--adopt-certbot=" + le, "--dns-credential-file=" + cred}
	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active"}
	useSystemd(t, sd)

	hook := filepath.Join(le, "renewal-hooks", "deploy", "00-copy-cert.sh")
	// prod's layout: the project is nself-web/backend, nginx serves nself-web/ssl, and the hook copies there
	acmeWriteFile(t, hook, []byte("#!/bin/sh\ncp \"$RENEWED_LINEAGE\"/fullchain.pem "+filepath.Join(f.ssl, "certificates", "api-task-nself-org")+"/\n"))
	_ = os.Chmod(hook, 0o755)
	_, err := f.run(sslSetupCmd, adopt...)
	if lf, _ := acme.Load(f.ssl); err == nil || !strings.Contains(err.Error(), hook+":2") || len(lf.Lineages) != 0 || len(f.set) != 0 || sd.disables() != 0 {
		t.Fatalf("saved hook: err=%v lineages=%d secrets=%d disables=%d (want a refusal that writes nothing)", err, len(lf.Lineages), len(f.set), sd.disables())
	}
	_ = os.Remove(hook)

	out, err := f.run(sslSetupCmd, append(adopt, "--dry-run")...)
	if err != nil || !strings.Contains(out, "would disable "+certbotTimer) || sd.disables() != 0 {
		t.Fatalf("dry run: err=%v disables=%d\n%s", err, sd.disables(), out)
	}

	sd.disErr = errors.New("exit status 1")
	_, err = f.run(sslSetupCmd, adopt...)
	if lf, _ := acme.Load(f.ssl); err == nil || !strings.Contains(err.Error(), "systemctl disable --now "+certbotTimer) || len(lf.Lineages) != 0 || len(f.set) != 0 {
		t.Fatalf("refused disable: err=%v lineages=%d secrets=%d (want the fix hint and nothing written)", err, len(lf.Lineages), len(f.set))
	}

	// The tree certbot runs from is checked too, even when adopting from a clean copy.
	sd.disErr, sd.calls = nil, nil
	live := useCertbotDir(t)
	acmeWriteFile(t, filepath.Join(live, "renewal", "api.task.nself.org.conf"), []byte("[renewalparams]\n"+nselfDeployHook(f.project)))
	if _, err = f.run(sslSetupCmd, adopt...); err == nil || !strings.Contains(err.Error(), live) || sd.disables() != 0 {
		t.Fatalf("hook in the live certbot tree: err=%v disables=%d", err, sd.disables())
	}
	_ = os.RemoveAll(filepath.Join(live, "renewal"))

	sd.disErr = nil
	if out, err = f.run(sslSetupCmd, adopt...); err != nil || !strings.Contains(out, "disabled "+certbotTimer) || !strings.Contains(out, "no renewal is scheduled") || sd.file != "disabled" || f.timers != 0 {
		t.Fatalf("adopt: err=%v file=%s timers=%d\n%s", err, sd.file, f.timers, out)
	}
	if out, err = f.run(sslSetupCmd, append(adopt, "--install-cron")...); err != nil || f.timers != 1 || strings.Contains(out, "no renewal is scheduled") {
		t.Fatalf("adopt --install-cron: err=%v timers=%d\n%s", err, f.timers, out)
	}
}

// TestACMEIssueRetiresCertbotTimer: a new lineage from `setup --acme` (no
// --install-cron) disables the certbot timer before issuing; a refused disable
// issues nothing.
func TestACMEIssueRetiresCertbotTimer(t *testing.T) {
	f := newACMEFix(t)
	useCertbotDir(t)
	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active", disErr: errors.New("exit status 1")}
	useSystemd(t, sd)
	if _, err := f.run(sslSetupCmd, "--acme", "--agree-tos"); err == nil || len(f.runs) != 0 {
		t.Fatalf("refused disable: err=%v lego runs=%d (want a refusal before issuing)", err, len(f.runs))
	}
	sd.disErr, sd.calls = nil, nil
	out, err := f.run(sslSetupCmd, "--acme", "--agree-tos")
	if lf, _ := acme.Load(f.ssl); err != nil || len(lf.Lineages) != 1 || sd.disables() != 1 || !strings.Contains(out, "disabled "+certbotTimer) {
		t.Fatalf("issue: err=%v lineages=%d disables=%d\n%s", err, len(lf.Lineages), sd.disables(), out)
	}
}

// TestACMEIssueRechecksHooksAfterRetire: a certbot renewal that saves its
// deploy hook between the first scan and the retirement is caught by the scan
// after it; nothing is issued and the operator is told the timer is now off.
func TestACMEIssueRechecksHooksAfterRetire(t *testing.T) {
	f := newACMEFix(t)
	le := useCertbotDir(t)
	hook := filepath.Join(le, "renewal", "x.conf")
	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active",
		onDisable: func() { acmeWriteFile(t, hook, []byte("[renewalparams]\n"+nselfDeployHook(f.project))) }}
	useSystemd(t, sd)
	out, err := f.run(sslSetupCmd, "--acme", "--agree-tos")
	if err == nil || codeOf(err) != "E151" || !strings.Contains(err.Error(), hook+":2") || len(f.runs) != 0 || strings.Count(out, "was disabled before this failure") != 1 {
		t.Fatalf("hook saved mid-retire: err=%v lego runs=%d\n%s", err, len(f.runs), out)
	}
}

// TestCertbotSetupRefusesUnderACME: the v1.4 certbot `ssl setup` refuses before
// running certbot in a project whose certificates ACME manages, or whose
// lineages.json cannot be read; without lineages it runs as before.
func TestCertbotSetupRefusesUnderACME(t *testing.T) {
	log := certbotStubs(t)
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
	state := filepath.Join(project, "ssl", ".acme", "lineages.json")

	for _, tc := range []struct {
		name, lineages, wantErr string
		wantCrons               int
		certbotRuns             bool
	}{
		{name: "no ACME state", wantCrons: 1, certbotRuns: true},
		{name: "ACME lineages", lineages: `{"schema_version":"1","lineages":[{"name":"example-org","targets":["certificates/example-org"]}]}`, wantErr: "ACME engine manages"},
		{name: "unreadable lineages.json", lineages: "{", wantErr: "reading the ACME lineages"},
	} {
		_ = os.Remove(log)
		if tc.lineages != "" {
			acmeWriteFile(t, state, []byte(tc.lineages))
		}
		crons = 0
		acmeResetFlags(sslSetupCmd)
		_ = sslSetupCmd.Flags().Set("install-cron", "true")
		err := runSSLSetup(sslSetupCmd, nil)
		if (err == nil) != (tc.wantErr == "") || (err != nil && !strings.Contains(err.Error(), tc.wantErr)) {
			t.Fatalf("%s: err=%v, want %q", tc.name, err, tc.wantErr)
		}
		if crons != tc.wantCrons || (readLog(log) != "") != tc.certbotRuns {
			t.Errorf("%s: certbot timer installs=%d (want %d), certbot calls=%q", tc.name, crons, tc.wantCrons, readLog(log))
		}
	}
}

// TestACMERetiredNoteSurvivesTimerFailure: adopt --install-cron disables the
// certbot timer, then a failed ACME timer install still says nothing renews;
// a staging dry run never offers to disable it.
func TestACMERetiredNoteSurvivesTimerFailure(t *testing.T) {
	f := newACMEFix(t)
	le := adoptFix(t, f)
	cred := filepath.Join(f.root, "cloudflare.ini")
	acmeWriteFile(t, cred, []byte("# certbot\ndns_cloudflare_api_token = fake-token-not-a-secret\n"))
	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active"}
	useSystemd(t, sd)
	acmeD.timer = func(string, string) error { return errors.New("systemd timers need Linux") }
	_, err := f.run(sslSetupCmd, "--acme", "--adopt-certbot="+le, "--dns-credential-file="+cred, "--install-cron")
	if err == nil || !strings.Contains(err.Error(), "nothing renews") || sd.file != "disabled" {
		t.Fatalf("err=%v file=%s (want the ACME timer failure to say nothing renews)", err, sd.file)
	}

	g := newACMEFix(t)
	sd.file, sd.active, sd.calls = "enabled", "active", nil
	out, err := g.run(sslSetupCmd, "--acme", "--agree-tos", "--staging", "--dry-run")
	if err != nil || strings.Contains(out, "would disable") || len(sd.calls) != 0 {
		t.Fatalf("staging dry run: err=%v calls=%v\n%s", err, sd.calls, out)
	}
}
