package entitlement

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/nself-org/cli/internal/license"
)

// Purpose: keep a paid online stack entitled with no operator command (P7-PLUG-64, review R1).
// Inputs: project root and the installed licensed plugins. Outputs: an idempotent host timer
// that runs `nself license refresh --project DIR --quiet`, and the scheduled refresh itself.
// Constraints: v1.5 only; the signed exp stays the only time anchor; a failed refresh never
// removes the timer or touches the old file; no key, JWT or `sub` in any output.
// SPORT: cap:cli.entitlement-autorefresh.

// Timers is the host timer API (internal/maintenance), supplied by the command layer so this
// package adds no same-layer import edge (repoqa D3). With no functions set no timer is managed.
type Timers struct {
	Ensure func(projectDir, bin string) (TimerInstall, error)
	Remove func(projectDir string) (TimerRemoval, error)
}

// TimerInstall is the result of Timers.Ensure.
type TimerInstall struct {
	Backend string
	Changed bool
}

// TimerRemoval is the result of Timers.Remove; RootRequired means root-owned systemd units remain.
type TimerRemoval struct{ Removed, RootRequired bool }

// Errors a Timers.Ensure may wrap so the build notice can name the reason.
var (
	ErrTimerUnsafePath = errors.New("unsafe-path")
	ErrTimerNoBackend  = errors.New("no-backend")
)

// Seams: the host timers (wired by cmd/commands), the clock, the running binary and the notice sink.
var (
	HostTimers   Timers
	refreshClock license.Clock // nil = wall clock; the simulation injects a fake one
	selfBinary                 = defaultSelfBinary
	timerNotes   io.Writer     = os.Stderr
)

// AutorefreshDisabled reports NSELF_ENTITLEMENT_AUTOREFRESH=false (also 0, no, off); the same
// rule as maintenance.AutorefreshDisabled, which TestAutorefreshDisabledParity keeps equal.
func AutorefreshDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("NSELF_ENTITLEMENT_AUTOREFRESH"))) {
	case "false", "0", "no", "off":
		return true
	}
	return false
}

func ensureTimer(dir, bin string) (TimerInstall, error) {
	if HostTimers.Ensure == nil {
		return TimerInstall{}, ErrTimerNoBackend
	}
	return HostTimers.Ensure(dir, bin)
}

func removeTimer(dir string) (TimerRemoval, error) {
	if HostTimers.Remove == nil {
		return TimerRemoval{}, nil
	}
	return HostTimers.Remove(dir)
}

// Failure classes printed by the scheduled refresh (`entitlement refresh failed: <class>`).
const (
	ClassPingUnavailable = "ping-unavailable"
	ClassRejected        = "licence-rejected"
	ClassTokenVerify     = "token-verify"
	ClassNoLicence       = "no-licence"
	ClassNotCovered      = "not-covered"
	ClassWriteFailed     = "write-failed"
	ClassInspect         = "plugin-inspect"
)

// RefreshError is a scheduled refresh failure; Error never carries more than the class.
type RefreshError struct {
	Class string
	Err   error
}

func (e *RefreshError) Error() string { return "entitlement refresh failed: " + e.Class }
func (e *RefreshError) Unwrap() error { return e.Err }

// Outcome is the result of one scheduled refresh.
type Outcome struct {
	Exp          time.Time // new token expiry (zero when nothing was refreshed)
	Removed      bool      // the timer's reason is gone and it was removed
	RootRequired bool      // systemd units remain; the caller exits 78 (maintenance.ExitGone)
	Skipped      bool      // v1.4 mode or development keys: nothing to do
	Reason       string    // why the timer was removed
}

func defaultSelfBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(strings.TrimSuffix(filepath.Base(exe), ".exe"), ".test") {
		return "", errors.New("go test binary")
	}
	return filepath.EvalSymlinks(exe)
}

// defaultPluginDir reports whether the plugin directory is the one the timer (which carries no
// environment) will see: ~/.nself/plugins.
func defaultPluginDir(pluginDir string) bool {
	dir := strings.TrimSpace(pluginDir)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv("NSELF_PLUGIN_DIR"))
	}
	home, err := os.UserHomeDir()
	return dir == "" || (err == nil && filepath.Clean(dir) == filepath.Join(home, ".nself", "plugins"))
}

// TimerWanted reports whether this host should run the refresh timer: v1.5, a licensed plugin
// in the default plugin directory and autorefresh not disabled. Used by doctor (E132).
func TimerWanted() bool {
	// compat.V15(P7-PLUG-64): no unattended refresh -> doctor expects a timer for licensed stacks
	if !compat.V15() || DevKeyMode() || AutorefreshDisabled() || !defaultPluginDir("") {
		return false
	}
	installed, err := HasLicensedInstalled()
	return err == nil && installed
}

// reconcileTimer ensures the timer when licensed plugins are installed and removes it when none
// remain. It is called after every build/start/license-set refresh and never fails them.
func reconcileTimer(projectDir string, names []string, pluginDir string) {
	// compat.V15(P7-PLUG-64): refresh only on build, start and license set -> idempotent unattended timer
	if !compat.V15() || DevKeyMode() || !defaultPluginDir(pluginDir) {
		return
	}
	if len(names) == 0 || AutorefreshDisabled() {
		if res, err := removeTimer(projectDir); err == nil && res.Removed {
			note("entitlement refresh timer removed")
		}
		return
	}
	bin, err := selfBinary()
	if err != nil {
		return
	}
	res, err := ensureTimer(projectDir, bin)
	switch {
	case errors.Is(err, ErrTimerUnsafePath):
		note("warning: entitlement refresh timer not installed (unsafe-path); nself doctor reports E132")
	case err != nil:
		note("warning: entitlement refresh timer not installed (" + installFailure(err) + "); nself doctor reports E132")
	case res.Changed:
		note("entitlement refresh timer installed (" + res.Backend + ", every 6 h; opt out with NSELF_ENTITLEMENT_AUTOREFRESH=false)")
	}
}

func installFailure(err error) string {
	if errors.Is(err, ErrTimerNoBackend) {
		return "no-backend"
	}
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return "install-failed: " + msg
}

func note(line string) { _, _ = fmt.Fprintln(timerNotes, line) }

// RunScheduled is one unattended refresh: it removes the timer when no licensed plugin remains
// or autorefresh is disabled, otherwise it revalidates online and rewrites the entitlement file
// in place only after the new JWT verifies and covers every installed licensed plugin.
func RunScheduled(ctx context.Context, projectDir string) (Outcome, error) {
	// compat.V15(P7-PLUG-64): no scheduled refresh -> scheduled revalidation and self-removal
	if !compat.V15() || DevKeyMode() {
		return Outcome{Skipped: true}, nil
	}
	names, err := installedLicensedNames("")
	if err != nil {
		return Outcome{}, &RefreshError{Class: ClassInspect, Err: err}
	}
	if len(names) == 0 || AutorefreshDisabled() {
		reason := "no licensed plugin installed"
		if AutorefreshDisabled() {
			reason = "autorefresh disabled"
		}
		res, _ := removeTimer(projectDir)
		return Outcome{Removed: res.Removed, RootRequired: res.RootRequired, Reason: reason}, nil
	}
	renewed, rejected, class := revalidateClassified(ctx)
	if len(renewed) == 0 {
		return Outcome{}, &RefreshError{Class: class}
	}
	token, err := selectToken(renewed, names, rejected)
	if err != nil {
		return Outcome{}, &RefreshError{Class: ClassNotCovered, Err: err}
	}
	if err := writeAtomic(Path(projectDir), []byte(token)); err != nil {
		return Outcome{}, &RefreshError{Class: ClassWriteFailed, Err: err}
	}
	claims, err := license.ParseLicenseJWT(token)
	if err != nil {
		return Outcome{}, &RefreshError{Class: ClassTokenVerify, Err: err}
	}
	return Outcome{Exp: time.Unix(claims.Exp, 0).UTC()}, nil
}

// revalidateClassified renews every configured key once and names why nothing was renewed.
func revalidateClassified(ctx context.Context) (renewed []string, rejected map[string]bool, class string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rejected, class = map[string]bool{}, ClassNoLicence
	for _, key := range license.CollectLicenseKeys() {
		remote := &validationStatus{client: httptimeout.Default}
		result, err := license.Validate(ctx, key, &license.ValidatorOptions{HTTPClient: remote, Clock: refreshClock})
		switch {
		case err == nil && explicitRejection(remote, result):
			rejected[license.HashKey(key)], class = true, ClassRejected
		case err == nil && result != nil && result.CanProceed && !result.FromCache && result.Status == license.StatusValid:
			if token, terr := cachedToken(); terr == nil {
				renewed = append(renewed, token)
			} else if class != ClassRejected {
				class = ClassTokenVerify
			}
		case remote.status == http.StatusOK && class != ClassRejected: // a 200 whose signature or JWT did not verify
			class = ClassTokenVerify
		case class == ClassNoLicence:
			class = ClassPingUnavailable
		}
	}
	return renewed, rejected, class
}

// PublishFromCache rewrites the entitlement file from the verified licence cache without
// network access (air-gapped `license import`). It does nothing in v1.4 mode or when no
// licensed plugin is installed.
func PublishFromCache(projectDir string) error {
	// compat.V15(P7-PLUG-64): import ignores the JWT -> import rewrites the entitlement file
	if !compat.V15() || DevKeyMode() {
		return nil
	}
	names, err := installedLicensedNames("")
	if err != nil || len(names) == 0 {
		return err
	}
	return publishToken(projectDir, names)
}
