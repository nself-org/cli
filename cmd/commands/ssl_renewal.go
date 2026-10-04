package commands

// Purpose: Builds and installs the certbot auto-renewal hook — a systemd
// service+timer unit on Linux, falling back to a cron entry — that
// reinstalls a renewed certificate where nginx reads it (mirroring
// installIssuedCert) whenever certbot renews a lineage. Split out of
// ssl_setup.go (CLI-R12) to separate the renewal-hook installers from the
// `ssl setup`/`ssl add` command handlers (ssl_setup.go, ssl_add.go) and
// the cert-install/nginx-config helpers (ssl_install.go).
// Inputs: the project workdir.
// Outputs: an installed systemd service+timer (installSSLRenewalSystemd)
// or cron entry (installSSLRenewalCron).
// Constraints: pure move — no behavior changes.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// sslRenewalServiceUnit builds the systemd service unit for certbot renewal,
// rooted at workdir.
//
// Two things this has to get right, both of which were previously wrong:
//
//   - WorkingDirectory. The reload runs `docker compose exec`, which can only
//     find the project's compose file from the project root. Without it the
//     post-hook silently failed and nginx kept the old certificate loaded.
//
//   - deploy-hook. `certbot renew` refreshes /etc/letsencrypt/live/<domain>/
//     but nothing copied the result into ssl/certificates/<domain-safe>/, so a
//     renewed certificate never reached nginx and the served cert would simply
//     expire ~90 days after issue. The hook mirrors installIssuedCert: it runs
//     only for lineages that actually renewed, derives the same dash-safe
//     directory name, and installs both files 0600.
func sslRenewalServiceUnit(workdir string) string {
	deployHook := fmt.Sprintf(
		`safe=$(basename "$RENEWED_LINEAGE" | tr '.:' '--'); `+
			`dest=%s/ssl/certificates/$safe; `+
			`mkdir -p "$dest" && `+
			`install -m 600 "$RENEWED_LINEAGE/fullchain.pem" "$RENEWED_LINEAGE/privkey.pem" "$dest/"`,
		workdir)

	return fmt.Sprintf(`[Unit]
Description=nself SSL certificate renewal
After=network.target

[Service]
Type=oneshot
WorkingDirectory=%s
ExecStart=/usr/bin/certbot renew --quiet --deploy-hook "%s" --post-hook "docker compose exec nginx nginx -s reload"
`, workdir, deployHook)
}

// sslRenewalTimerContent is the systemd timer unit that triggers renewal twice daily.
const sslRenewalTimerContent = `[Unit]
Description=nself SSL certificate renewal timer

[Timer]
OnCalendar=*-*-* 03:30:00
RandomizedDelaySec=3600
Persistent=true

[Install]
WantedBy=timers.target
`

// installSSLRenewalCron installs automatic Let's Encrypt certificate renewal.
// On Linux with systemd, creates and enables a systemd timer unit.
// Returns an error with crontab fallback instructions if installation fails.
func installSSLRenewalCron(workdir string) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemd not available — add to crontab: 30 3 * * * certbot renew --quiet")
	}
	return installSSLRenewalSystemd(workdir)
}

// systemdUnitDir is where unit files are written; a variable so tests can redirect it.
var systemdUnitDir = "/etc/systemd/system"

// installSSLRenewalSystemd writes and enables the nself-ssl-renew systemd timer.
func installSSLRenewalSystemd(workdir string) error {
	return installSystemdTimer("nself-ssl-renew", sslRenewalServiceUnit(workdir), sslRenewalTimerContent)
}

// installSystemdTimer writes <name>.service and <name>.timer into the unit
// directory, reloads systemd and enables the timer.
func installSystemdTimer(name, service, timer string) error {
	servicePath := filepath.Join(systemdUnitDir, name+".service")
	if err := os.WriteFile(servicePath, []byte(service), 0644); err != nil {
		return fmt.Errorf("writing service unit: %w", err)
	}

	timerPath := filepath.Join(systemdUnitDir, name+".timer")
	if err := os.WriteFile(timerPath, []byte(timer), 0644); err != nil {
		return fmt.Errorf("writing timer unit: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := exec.CommandContext(ctx, "systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()

	if err := exec.CommandContext(ctx2, "systemctl", "enable", "--now", name+".timer").Run(); err != nil {
		return fmt.Errorf("enable %s.timer: %w", name, err)
	}

	return nil
}

// The ACME renewal timer: unit text, the Linux-only installer and the acmeRun step
// (`setup --acme --install-cron`). Environment lines are explicit because
// systemd gives a service neither HOME nor the age key path.

// acmeUnits renders the renewal service and timer units.
func acmeUnits(exe, workdir, keyPath, home string) (service, timer string) {
	return fmt.Sprintf(`[Unit]
Description=nself ACME certificate renewal
After=network.target docker.service

[Service]
Type=oneshot
WorkingDirectory=%s
Environment="SECRETS_AGE_KEY_PATH=%s"
Environment="HOME=%s"
ExecStart=%s trust ssl renew --acme --quiet
`, workdir, keyPath, home, exe), sslRenewalTimerContent
}

// acmeUnitsFor renders the units for this binary, project and age key.
func acmeUnitsFor(workdir, keyPath string) (service, timer string, err error) {
	exe, err := os.Executable()
	if runtime.GOOS != "linux" && err == nil {
		err = fmt.Errorf("systemd timers need Linux")
	}
	home, _ := os.UserHomeDir()
	abs, _ := filepath.Abs(keyPath)
	service, timer = acmeUnits(exe, workdir, abs, home)
	return service, timer, err
}

// timer installs the renewal units; under --dry-run it prints them instead.
func (r *acmeRun) timer() error {
	if r.dry {
		service, timer, _ := acmeUnitsFor(r.workdir, r.res.AgeKey)
		r.say("dry run: would write nself-acme-renew.service:\n%s\nand nself-acme-renew.timer:\n%s", service, timer)
		return nil
	}
	if err := acmeD.timer(r.workdir, r.res.AgeKey); err != nil {
		return e151(acmeRefuse("add to cron: 30 3 * * * nself trust ssl renew --acme --quiet", "installing the renewal timer: %v", err))
	}
	r.say("renewal timer installed (nself-acme-renew.timer, runs as root with age key %s)", r.res.AgeKey)
	return nil
}

// installACMETimer writes and enables nself-acme-renew.{service,timer} (Linux, systemd).
func installACMETimer(workdir, keyPath string) error {
	service, timer, err := acmeUnitsFor(workdir, keyPath)
	if err != nil {
		return err
	}
	return installSystemdTimer("nself-acme-renew", service, timer)
}
