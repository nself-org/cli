package commands

// Purpose: retires the certbot renewal timer once the ACME engine owns the
// certificates (D-0411). The timer `ssl setup --install-cron` installs runs a
// deploy hook that copies each renewed certbot pair into ssl/certificates/<safe>.
// Under ACME that path is the live generation symlink, so the copy would
// overwrite the ACME pair in place, outside the ACME lock. Installing the ACME
// timer or adopting a lineage disables it.
// Inputs: systemd state through systemctlRun (a seam tests replace).
// Outputs: a disabled and stopped nself-ssl-renew.timer, or a refusal with the
// manual command.
// Constraints: idempotent (state is read before any change); never touches
// certbot's own files.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// certbotTimer is the certbot renewal timer unit installSSLRenewalSystemd enables.
const certbotTimer = "nself-ssl-renew.timer"

// errNoSystemctl means systemctl is not on PATH, so no systemd timer can run.
var errNoSystemctl = errors.New("systemctl not found")

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
// systemctl, an unknown unit, or a disabled or masked inactive unit is not live.
// When systemd cannot answer, only a unit file on disk counts as live.
func certbotTimerLive(ctx context.Context) (bool, error) {
	out, err := systemctlRun(ctx, "show", certbotTimer, "-p", "LoadState", "-p", "UnitFileState", "-p", "ActiveState")
	if errors.Is(err, errNoSystemctl) {
		return false, nil
	}
	if err != nil {
		if _, serr := os.Stat(filepath.Join(systemdUnitDir, certbotTimer)); errors.Is(serr, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("systemctl show %s: %v %s", certbotTimer, err, strings.TrimSpace(out))
	}
	p := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			p[k] = v
		}
	}
	if p["LoadState"] == "" {
		return false, fmt.Errorf("systemctl show %s: unexpected output %q", certbotTimer, strings.TrimSpace(out))
	}
	off := p["LoadState"] == "not-found" || p["LoadState"] == "masked" ||
		p["UnitFileState"] == "disabled" || strings.HasPrefix(p["UnitFileState"], "masked")
	idle := p["ActiveState"] == "inactive" || p["ActiveState"] == "failed"
	return !(off && idle), nil
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

// retireCertbot runs retireCertbotTimer for an ACME run. Under --dry-run it
// only reports; a failure refuses with the manual command.
func (r *acmeRun) retireCertbot() error {
	ctx := context.Background()
	if r.dry {
		if live, err := certbotTimerLive(ctx); err != nil || live {
			r.say("dry run: would disable %s (its certbot deploy hook overwrites ACME-managed certificates)", certbotTimer)
		}
		return nil
	}
	done, err := retireCertbotTimer(ctx)
	if err != nil {
		return e151(acmeRefuse("run `sudo systemctl disable --now "+certbotTimer+"`, then re-run this command",
			"%s (certbot renewal) would overwrite ACME-managed certificates and could not be disabled: %v", certbotTimer, err))
	}
	if done {
		r.say("disabled %s: its certbot deploy hook would overwrite ACME-managed certificates", certbotTimer)
	}
	return nil
}
