package commands

// Purpose: retires certbot's writers once the ACME engine owns the
// certificates (D-0411). The timer `ssl setup --install-cron` installs runs a
// deploy hook that copies each renewed certbot pair into ssl/certificates/<safe>.
// Under ACME that path is the live generation symlink, so the copy would
// overwrite the ACME pair in place, outside the ACME lock. certbot also saves
// that hook in renewal/<lineage>.conf, where any certbot scheduler (the distro
// certbot.timer, snap, cron.d) replays it, and an operator's
// renewal-hooks/deploy script can do the same.
// The certbot hook scan lives in ssl_certbot_hooks.go.
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

// readUnit reads unit's state. An unknown unit is the zero state; when
// systemctl is missing from PATH or systemd cannot answer, only a unit file on
// disk is an error (a PATH without systemctl does not stop systemd). A masked
// unit (UnitFileState "masked") is not enabled but can still be active.
func readUnit(ctx context.Context, unit string) (unitState, error) {
	out, err := systemctlRun(ctx, "show", unit, "-p", "LoadState", "-p", "UnitFileState", "-p", "ActiveState")
	if err != nil {
		file := filepath.Join(systemdUnitDir, unit)
		if _, serr := os.Stat(file); errors.Is(serr, os.ErrNotExist) {
			return unitState{}, nil
		}
		if errors.Is(err, errNoSystemctl) {
			return unitState{}, fmt.Errorf("%w on PATH, but %s exists", errNoSystemctl, file)
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
	if err != nil || (!st.enabled && !st.active) {
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
		return done, e151(acmeRefuse("wait for it to finish (`systemctl status "+certbotService+"`), then re-run this command", "%v", err))
	}
	if err != nil {
		fix := "run `sudo systemctl disable --now " + certbotTimer + "` (and `sudo systemctl disable --runtime " + certbotTimer + "` if it was enabled at runtime), then re-run this command"
		switch {
		case errors.Is(err, errSystemdDown):
			fix = "remove " + filepath.Join(systemdUnitDir, "nself-ssl-renew.{timer,service}") + ", then re-run this command"
		case errors.Is(err, errNoSystemctl):
			fix = "re-run this command with systemctl on PATH (e.g. under sudo); if this host does not run systemd, remove " + filepath.Join(systemdUnitDir, "nself-ssl-renew.{timer,service}") + " first"
		}
		return false, e151(acmeRefuse(fix, "%s (certbot renewal) would overwrite ACME-managed certificates and could not be disabled: %v", certbotTimer, err))
	}
	return done, nil
}

// retireAndRecheck retires the certbot timer, then scans the certbot hooks
// again: a renewal that ran between the first scan and the retirement can have
// saved its deploy hook into renewal/<lineage>.conf, and once the timer is down
// nothing can add one (D-0411). The first scan still refuses before any change.
// already says an earlier step of the run disabled the timer, for the hint.
func (r *acmeRun) retireAndRecheck(already bool, dirs ...string) (bool, error) {
	retired, err := r.retireCertbot()
	if err != nil {
		return retired, err
	}
	if err := r.refuseCertbotHooks(dirs...); err != nil {
		r.retiredHint(already || retired)
		return retired, err
	}
	return retired, nil
}

// retiredHint tells the operator, when a run fails after it disabled the
// certbot timer, that nothing renews until they re-run or go back to certbot.
func (r *acmeRun) retiredHint(retired bool) {
	if retired {
		r.say("%s was disabled before this failure: re-run to finish, or `sudo systemctl enable --now %s` to go back to certbot renewal", certbotTimer, certbotTimer)
	}
}
