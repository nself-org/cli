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
	"cmp"
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

// certbotTimer and certbotService are the certbot renewal units
// installSSLRenewalSystemd writes.
const (
	certbotTimer   = "nself-ssl-renew.timer"
	certbotService = "nself-ssl-renew.service"
)

// certbotDir is the certbot tree checked for saved hooks; a variable so tests redirect it.
var certbotDir = "/etc/letsencrypt"

// errNoSystemctl means systemctl is not on PATH, so no systemd timer can run.
var errNoSystemctl = errors.New("systemctl not found")

// errRenewalRunning means the certbot service is in the middle of a renewal.
var errRenewalRunning = errors.New("is running a certbot renewal now")

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

// unitState is what systemd says about one unit.
type unitState struct{ enabled, active bool }

// readUnit reads unit's state. No systemctl or an unknown unit is the zero
// state; when systemd cannot answer, only a unit file on disk is an error. A
// masked unit (UnitFileState "masked") is not enabled but can still be active.
func readUnit(ctx context.Context, unit string) (unitState, error) {
	out, err := systemctlRun(ctx, "show", unit, "-p", "LoadState", "-p", "UnitFileState", "-p", "ActiveState")
	if errors.Is(err, errNoSystemctl) {
		return unitState{}, nil
	}
	if err != nil {
		if _, serr := os.Stat(filepath.Join(systemdUnitDir, unit)); errors.Is(serr, os.ErrNotExist) {
			return unitState{}, nil
		}
		return unitState{}, fmt.Errorf("%w: systemctl show %s: %v %s", errSystemdDown, unit, err, strings.TrimSpace(out))
	}
	p := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			p[k] = v
		}
	}
	if p["LoadState"] == "" {
		return unitState{}, fmt.Errorf("systemctl show %s: unexpected output %q", unit, strings.TrimSpace(out))
	}
	f := p["UnitFileState"]
	return unitState{
		enabled: strings.HasPrefix(f, "enabled") || f == "alias" || f == "indirect",
		active:  slices.Contains([]string{"active", "activating", "reloading", "deactivating"}, p["ActiveState"]),
	}, nil
}

// certbotTimerLive reports whether the certbot timer is enabled or running.
func certbotTimerLive(ctx context.Context) (bool, error) {
	st, err := readUnit(ctx, certbotTimer)
	return st.enabled || st.active, err
}

// retireCertbotTimer disables and stops the certbot timer when it is live
// (stops it when it is only running, e.g. masked), then re-checks it. It
// refuses while the certbot service is mid-run: stopping it could cut the
// copy in half. The state is read first, so a re-run changes nothing; it
// reports whether it changed the timer.
func retireCertbotTimer(ctx context.Context) (bool, error) {
	if svc, err := readUnit(ctx, certbotService); err != nil || svc.active {
		return false, cmp.Or(err, fmt.Errorf("%s %w", certbotService, errRenewalRunning))
	}
	st, err := readUnit(ctx, certbotTimer)
	if err != nil || !(st.enabled || st.active) {
		return false, err
	}
	args := []string{"stop", certbotTimer}
	if st.enabled {
		args = []string{"disable", "--now", certbotTimer}
	}
	if out, err := systemctlRun(ctx, args...); err != nil {
		return false, fmt.Errorf("systemctl %s: %v %s", strings.Join(args, " "), err, strings.TrimSpace(out))
	}
	if live, err := certbotTimerLive(ctx); err != nil || live {
		return false, cmp.Or(err, fmt.Errorf("%s is still enabled or running after disable", certbotTimer))
	}
	// the timer may have fired between the first check and the disable
	if svc, err := readUnit(ctx, certbotService); err != nil || svc.active {
		return true, cmp.Or(err, fmt.Errorf("%s %w (it started while the timer was being disabled)", certbotService, errRenewalRunning))
	}
	return true, nil
}

// certbotHookFiles lists where certbot reads hooks under dir: cli.ini and
// renewal/*.conf (confs), and the files it runs from renewal-hooks/{pre,deploy,post}
// (scripts: regular, executable, no trailing '~', dot files included). Only
// absence is tolerated; any other read error is returned.
func certbotHookFiles(dir string) (confs, scripts []string, err error) {
	list := func(d string) ([]string, error) {
		es, err := os.ReadDir(d)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		var out []string
		for _, e := range es {
			out = append(out, filepath.Join(d, e.Name()))
		}
		return out, err
	}
	regular := func(p string) (os.FileInfo, bool, error) {
		st, err := os.Stat(p)
		if errors.Is(err, os.ErrNotExist) { // dangling link: nothing to run
			return nil, false, nil
		}
		return st, err == nil && st.Mode().IsRegular(), err
	}
	renewal, err := list(filepath.Join(dir, "renewal"))
	if err != nil {
		return nil, nil, err
	}
	for _, p := range append([]string{filepath.Join(dir, "cli.ini")}, renewal...) {
		_, ok, err := regular(p)
		if err != nil {
			return nil, nil, err
		}
		if ok && (strings.HasSuffix(p, ".conf") || filepath.Base(p) == "cli.ini") {
			confs = append(confs, p)
		}
	}
	for _, sub := range []string{"pre", "deploy", "post"} {
		ps, err := list(filepath.Join(dir, "renewal-hooks", sub))
		if err != nil {
			return nil, nil, err
		}
		for _, p := range ps {
			st, ok, err := regular(p)
			if err != nil {
				return nil, nil, err
			}
			if ok && !strings.HasSuffix(p, "~") && (runtime.GOOS == "windows" || st.Mode()&0o111 != 0) {
				scripts = append(scripts, p)
			}
		}
	}
	return confs, scripts, nil
}

// certbotHooks lists the certbot hooks under dir that name one of sslDirs:
// "*_hook"/"*-hook" lines in cli.ini and renewal/*.conf, and non-comment
// lines of the scripts certbot runs. A missing tree has none.
func certbotHooks(dir string, sslDirs ...string) ([]string, error) {
	var needles []string
	for _, d := range sslDirs {
		needles = append(needles, filepath.Clean(d))
		if real, err := filepath.EvalSymlinks(d); err == nil && real != filepath.Clean(d) {
			needles = append(needles, real)
		}
	}
	confs, scripts, err := certbotHookFiles(dir)
	if err != nil {
		return nil, err
	}
	var hits []string
	for _, p := range append(confs, scripts...) {
		data, err := os.ReadFile(p) //nolint:gosec // certbot's tree, read only
		if err != nil {
			return nil, err
		}
		isConf := slices.Contains(confs, p)
		for i, line := range strings.Split(string(data), "\n") {
			k, _, _ := strings.Cut(line, "=")
			k = strings.TrimSpace(k)
			if (isConf && !strings.HasSuffix(k, "_hook") && !strings.HasSuffix(k, "-hook")) || strings.HasPrefix(strings.TrimSpace(line), "#") || (isConf && strings.HasPrefix(strings.TrimSpace(line), ";")) {
				continue
			}
			if slices.ContainsFunc(needles, func(n string) bool { return mentionsPath(line, n) }) {
				hits = append(hits, fmt.Sprintf("%s:%d", p, i+1))
			}
		}
	}
	return hits, nil
}

// mentionsPath reports whether line names dir or a path under it: dir starts
// at the line start or after a space, quote, '=' or ':', and ends at the line
// end or before a separator, space, quote, ';' or ')'.
func mentionsPath(line, dir string) bool {
	for off := 0; ; {
		i := strings.Index(line[off:], dir)
		if i < 0 {
			return false
		}
		at, end := off+i, off+i+len(dir)
		before := at == 0 || strings.ContainsRune(" \t\"'=:", rune(line[at-1]))
		after := end == len(line) || strings.ContainsRune(string(filepath.Separator)+"/ \t\"';)\r", rune(line[end]))
		if before && after {
			return true
		}
		off = at + 1
	}
}

// refuseCertbotHooks refuses (in every mode, before anything is written) while
// a certbot hook under one of dirs would copy certbot's files over
// ACME-managed ones: into the served ssl dir (NGINX_FRONTED_BY moves it out of
// the project) or the project's own ssl dir.
func (r *acmeRun) refuseCertbotHooks(dirs ...string) error {
	for i, d := range dirs {
		dirs[i] = filepath.Clean(d)
	}
	slices.Sort(dirs)
	for _, dir := range slices.Compact(dirs) {
		hits, err := certbotHooks(dir, r.res.SSLDir, filepath.Join(r.workdir, "ssl"))
		if err != nil {
			return e151(acmeRefuse("run as the user that owns "+dir, "cannot read certbot's hooks: %v", err))
		}
		if len(hits) > 0 {
			return e151(acmeRefuse("remove those hook lines and move those renewal-hooks scripts out of "+dir+", then re-run",
				"certbot would copy its certificates over the ACME-managed ones from: %s", strings.Join(hits, ", ")))
		}
	}
	return nil
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
	if done {
		r.say("disabled %s: its certbot deploy hook would overwrite ACME-managed certificates", certbotTimer)
	}
	if errors.Is(err, errRenewalRunning) {
		return done, e151(acmeRefuse("wait for it to finish (`systemctl status "+certbotService+"`), then re-run this command",
			"%v; nothing was written", err))
	}
	if err != nil {
		fix := "run `sudo systemctl disable --now " + certbotTimer + "` (and `sudo systemctl disable --runtime " + certbotTimer + "` if it was enabled at runtime), then re-run this command"
		if errors.Is(err, errSystemdDown) {
			fix = "remove " + filepath.Join(systemdUnitDir, "nself-ssl-renew.{timer,service}") + ", then re-run this command"
		}
		return false, e151(acmeRefuse(fix, "%s (certbot renewal) would overwrite ACME-managed certificates and could not be disabled: %v", certbotTimer, err))
	}
	return done, nil
}

// retiredHint tells the operator, when a run fails after it disabled the
// certbot timer, that nothing renews until they re-run or go back to certbot.
func (r *acmeRun) retiredHint(retired bool) {
	if retired {
		r.say("%s was disabled before this failure: re-run to finish, or `sudo systemctl enable --now %s` to go back to certbot renewal", certbotTimer, certbotTimer)
	}
}
