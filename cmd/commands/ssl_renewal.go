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
