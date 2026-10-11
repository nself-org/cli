package commands

// Purpose: retires certbot's writers once the ACME engine owns the
// certificates (D-0411). The timer `ssl setup --install-cron` installs runs a
// deploy hook that copies each renewed certbot pair into ssl/certificates/<safe>.
// Under ACME that path is the live generation symlink, so the copy would
// overwrite the ACME pair in place, outside the ACME lock. certbot also saves
// that hook in renewal/<lineage>.conf, where any certbot scheduler (the distro
// certbot.timer, snap, cron.d) replays it, and an operator's
// renewal-hooks/deploy script can do the same.
// Inputs: systemd state through systemctlRun and the certbot tree (seams tests replace).
// Outputs: a disabled and stopped nself-ssl-renew.timer; a refusal naming
// any certbot hook that writes into the project's ssl dir.
// Constraints: idempotent (state is read before any change); certbot's own
// files are only read, never edited.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// certbotTimer is the certbot renewal timer unit installSSLRenewalSystemd enables.
const certbotTimer = "nself-ssl-renew.timer"

// certbotDir is the certbot tree checked for saved hooks; a variable so tests redirect it.
var certbotDir = "/etc/letsencrypt"

// errNoSystemctl means systemctl is not on PATH, so no systemd timer can run.
var errNoSystemctl = errors.New("systemctl not found")

// errSystemdDown means systemctl cannot reach systemd but the unit file exists.
var errSystemdDown = errors.New("systemd is not answering")

// systemctlRun runs systemctl with args and returns its combined output; a
// variable so tests inject a fake (no real systemd in tests).
var systemctlRun = func(ctx context.Context, args ...string) (string, error) {
	bin, err := exec.LookPath("systemctl")
	if err != nil {
		return "", errNoSystemctl
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput() //nolint:gosec // systemctl from PATH, fixed verbs and unit name
	return string(out), err
}

// certbotTimerLive reports whether the certbot timer is enabled or running. No
// systemctl, an unknown or masked unit, or an idle unit that is not enabled is
// not live. When systemd cannot answer, only a unit file on disk counts as live.
func certbotTimerLive(ctx context.Context) (bool, error) {
	out, err := systemctlRun(ctx, "show", certbotTimer, "-p", "LoadState", "-p", "UnitFileState", "-p", "ActiveState")
	if errors.Is(err, errNoSystemctl) {
		return false, nil
	}
	if err != nil {
		if _, serr := os.Stat(filepath.Join(systemdUnitDir, certbotTimer)); errors.Is(serr, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("%w: systemctl show %s: %v %s", errSystemdDown, certbotTimer, err, strings.TrimSpace(out))
	}
	p := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			p[k] = v
		}
	}
	switch p["LoadState"] {
	case "":
		return false, fmt.Errorf("systemctl show %s: unexpected output %q", certbotTimer, strings.TrimSpace(out))
	case "not-found", "masked":
		return false, nil
	}
	switch p["ActiveState"] {
	case "active", "activating", "reloading", "deactivating":
		return true, nil
	}
	f := p["UnitFileState"]
	return strings.HasPrefix(f, "enabled") || f == "alias" || f == "indirect", nil
}

// retireCertbotTimer disables and stops the certbot timer when it is live, then
// re-checks it. The state is read first, so a re-run changes nothing; it
// reports whether it disabled the timer.
func retireCertbotTimer(ctx context.Context) (bool, error) {
	live, err := certbotTimerLive(ctx)
	if err != nil || !live {
		return false, err
	}
	if out, err := systemctlRun(ctx, "disable", "--now", certbotTimer); err != nil {
		return false, fmt.Errorf("systemctl disable --now %s: %v %s", certbotTimer, err, strings.TrimSpace(out))
	}
	if live, err = certbotTimerLive(ctx); err == nil && live {
		err = fmt.Errorf("%s is still enabled or running after disable", certbotTimer)
	}
	return err == nil, err
}

// certbotHooks lists the certbot hooks under dir that write into one of
// sslDirs: deploy_hook/renew_hook lines saved in renewal/*.conf and the
// executable scripts certbot runs from renewal-hooks/deploy (comment lines
// skipped). A missing tree has none.
func certbotHooks(dir string, sslDirs ...string) ([]string, error) {
	var needles []string
	for _, d := range sslDirs {
		needles = append(needles, filepath.Clean(d)+string(filepath.Separator))
		if real, err := filepath.EvalSymlinks(d); err == nil && real != filepath.Clean(d) {
			needles = append(needles, real+string(filepath.Separator))
		}
	}
	var hits []string
	confs, _ := filepath.Glob(filepath.Join(dir, "renewal", "*.conf"))
	scripts, _ := filepath.Glob(filepath.Join(dir, "renewal-hooks", "deploy", "*"))
	for _, p := range append(confs, scripts...) {
		isConf := strings.HasSuffix(p, ".conf") && filepath.Base(filepath.Dir(p)) == "renewal"
		st, err := os.Stat(p)
		if err != nil || st.IsDir() || (!isConf && runtime.GOOS != "windows" && st.Mode()&0o111 == 0) {
			continue // gone since the glob, a directory, or a script certbot does not run
		}
		data, err := os.ReadFile(p) //nolint:gosec // certbot's tree, read only
		if err != nil {
			return nil, err
		}
		for i, line := range strings.Split(string(data), "\n") {
			k, _, _ := strings.Cut(line, "=")
			k = strings.TrimSpace(k)
			if (isConf && k != "deploy_hook" && k != "renew_hook") || (!isConf && strings.HasPrefix(strings.TrimSpace(line), "#")) {
				continue
			}
			if slices.ContainsFunc(needles, func(n string) bool { return mentionsPath(line, n) }) {
				hits = append(hits, fmt.Sprintf("%s:%d", p, i+1))
			}
		}
	}
	return hits, nil
}

// mentionsPath reports whether dir (ending in a separator) starts a path in
// line: at the line start or after a space, quote, '=' or ':'.
func mentionsPath(line, dir string) bool {
	for off := 0; ; {
		i := strings.Index(line[off:], dir)
		if i < 0 {
			return false
		}
		if at := off + i; at == 0 || strings.ContainsRune(" \t\"'=:", rune(line[at-1])) {
			return true
		}
		off += i + 1
	}
}

// refuseCertbotHooks refuses (in every mode, before anything is written) while
// a certbot hook under dir would copy certbot's files over ACME-managed ones:
// into the served ssl dir (NGINX_FRONTED_BY moves it out of the project) or
// the project's own ssl dir.
func (r *acmeRun) refuseCertbotHooks(dir string) error {
	hits, err := certbotHooks(dir, r.res.SSLDir, filepath.Join(r.workdir, "ssl"))
	if err != nil {
		return e151(acmeRefuse("run as the user that owns "+dir, "cannot read certbot's hooks: %v", err))
	}
	if len(hits) == 0 {
		return nil
	}
	return e151(acmeRefuse("delete those deploy_hook/renew_hook lines and move those renewal-hooks/deploy scripts out of "+dir+", then re-run",
		"certbot would copy its certificates over the ACME-managed ones from: %s", strings.Join(hits, ", ")))
}

// retireCertbot runs retireCertbotTimer for an ACME run and reports whether it
// disabled the timer. Under --dry-run it only reports; a failure refuses with
// the manual step.
func (r *acmeRun) retireCertbot() (bool, error) {
	ctx := context.Background()
	if r.dry {
		if live, err := certbotTimerLive(ctx); err != nil {
			r.say("dry run: cannot read the state of %s: %v", certbotTimer, err)
		} else if live {
			r.say("dry run: would disable %s (its certbot deploy hook overwrites ACME-managed certificates)", certbotTimer)
		}
		return false, nil
	}
	done, err := retireCertbotTimer(ctx)
	if err != nil {
		fix := "run `sudo systemctl disable --now " + certbotTimer + "` (and `sudo systemctl disable --runtime " + certbotTimer + "` if it was enabled at runtime), then re-run this command"
		if errors.Is(err, errSystemdDown) {
			fix = "remove " + filepath.Join(systemdUnitDir, "nself-ssl-renew.{timer,service}") + ", then re-run this command"
		}
		return false, e151(acmeRefuse(fix, "%s (certbot renewal) would overwrite ACME-managed certificates and could not be disabled: %v", certbotTimer, err))
	}
	if done {
		r.say("disabled %s: its certbot deploy hook would overwrite ACME-managed certificates", certbotTimer)
	}
	return done, nil
}
