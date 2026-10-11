package commands

// Purpose: after `nself plugin install` of a plugin that declares
// migrations.apply: boot, wait for its migrations to be applied (P7-PLUG-19,
// contract:plugin.boot-migrations). The plugin applies its own SQL at boot; the
// CLI only waits on /health.
// Inputs: the plugin directory and the installed official plugin names.
// Outputs: nil, or the readiness error (E118/E119, exit 2) in v1.5; in v1.4 the
// same error is printed as a warning and nil is returned.
// After readiness (v1.5 only, P7-PLUG-78) the installed plugins' manifest tables
// are tracked in Hasura with one targeted bulk call and then verified; a failure
// is E140 (exit 2) naming the plugin and its untracked tables.
// Constraints: only inside a project; v1.5 waits only when the stack is running
// (it starts the stack first so the new container exists), otherwise it says
// readiness is checked at the next `nself start` / `nself doctor`; v1.4 never
// starts anything and waits only for a plugin that already answers /health.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/readiness"

	"github.com/spf13/cobra"
)

// Seams for tests: no Docker, no stack start, short waits.
var (
	pluginStartStack   = bundleStartStack
	pluginStackRunning = stackRunning
	pluginWait         = readiness.Wait
	pluginReadyEvery   time.Duration // 0 = readiness default
	pluginTrack        = pluginInstallTracking
	pluginStackState   = stackRunningState // tracking step: a docker error is not "stack down"
)

// stackRunningState reports whether the project has a running container, and
// the docker error when it could not tell (docker ps failing is not "stack down").
func stackRunningState(ctx context.Context, projectName string) (bool, error) {
	svcs, err := docker.RunningConfigHashes(ctx, projectName)
	if err != nil {
		return false, err
	}
	for _, s := range svcs {
		if s.State == "running" {
			return true, nil
		}
	}
	return false, nil
}

// stackRunning is the readiness check's view (unchanged since P7-PLUG-19): a
// docker error reads as "not running" there.
func stackRunning(ctx context.Context, projectName string) bool {
	ok, err := stackRunningState(ctx, projectName)
	return err == nil && ok
}

// officialInstalledNames keeps the official installs that have a directory here.
// A name with no directory is not installed here: nothing to wait on or track.
// Anything else that cannot be read is a real problem for the caller.
func officialInstalledNames(pluginDir string, installed []string) []string {
	var names []string
	for _, n := range installed {
		if plugin.IsThirdPartyInstallSource(n) {
			continue
		}
		if _, err := os.Stat(filepath.Join(pluginDir, n)); err == nil {
			names = append(names, n)
		}
	}
	return names
}

// pluginInstallReadiness waits for boot migrations of the installed plugins,
// then (v1.5) tracks their manifest tables in Hasura (below).
func pluginInstallReadiness(cmd *cobra.Command, pluginDir string, installed []string) error {
	if err := waitInstalledMigrations(cmd, pluginDir, installed); err != nil {
		return err
	}
	return pluginTrack(cmd, pluginDir, installed)
}

// waitInstalledMigrations is the readiness half of pluginInstallReadiness.
func waitInstalledMigrations(cmd *cobra.Command, pluginDir string, installed []string) error {
	names := officialInstalledNames(pluginDir, installed)
	stderr := cmd.ErrOrStderr()
	members, err := appready.LoadMembers(pluginDir, names)
	if err != nil {
		return readinessIssue(fmt.Errorf("reading the installed plugin manifests for the readiness check: %w", err), stderr)
	}
	var boot []appready.Member
	var noPort []string
	for _, m := range members {
		switch {
		case m.BootMigrations && m.Port > 0:
			boot = append(boot, m)
		case m.BootMigrations:
			noPort = append(noPort, m.Name)
		}
	}
	if len(boot) == 0 && len(noPort) == 0 {
		return nil
	}
	cfg, _, err := bundleProjectConfig()
	if err != nil {
		if errors.Is(err, appready.ErrNoProject) {
			return nil // outside a project there is no stack to wait on
		}
		return readinessIssue(fmt.Errorf("loading the project config for the readiness check: %w", err), stderr)
	}
	// Same rule as the bundle flow: a boot plugin without a port cannot be probed.
	for _, n := range noPort {
		if err := readinessIssue(fmt.Errorf("plugin %s: plugin.json declares no port, so migrations readiness cannot be probed", n), stderr); err != nil {
			return err
		}
	}
	if len(boot) == 0 {
		return nil
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	timeout, terr := readiness.TimeoutFromEnv()
	if terr != nil {
		timeout = readiness.DefaultTimeout
	}
	// compat.V15(P7-PLUG-19): plugin install readiness failure warns -> E118 exit 2
	if compat.V15() {
		if !pluginStackRunning(ctx, cfg.ProjectName) {
			_, _ = fmt.Fprintln(stderr, "note: the stack is not running; readiness of "+strings.Join(memberNames(boot), ", ")+" is checked at the next `nself start` / `nself doctor`")
			return nil
		}
		if err := pluginStartStack(ctx); err != nil {
			return fmt.Errorf("starting the stack for %s: %w", strings.Join(memberNames(boot), ", "), err)
		}
		return waitBootMigrations(ctx, boot, timeout, false, stderr)
	}
	return waitBootMigrations(ctx, boot, timeout, true, stderr)
}

// readinessIssue is a problem that stops the readiness check from running:
// returned in v1.5, printed as a warning in v1.4.
func readinessIssue(err error, stderr io.Writer) error {
	// compat.V15(P7-PLUG-19): plugin install readiness setup problems warn -> returned error
	if compat.V15() {
		return err
	}
	_, _ = fmt.Fprintf(stderr, "warning: %v\n", err)
	return nil
}

// waitBootMigrations waits for each member. warnOnly (v1.4) skips a plugin that
// does not answer /health yet and prints failures instead of returning them.
func waitBootMigrations(ctx context.Context, boot []appready.Member, timeout time.Duration, warnOnly bool, stderr io.Writer) error {
	deadline := time.Now().Add(timeout) // one bound for all members, not per member
	for _, m := range boot {
		url := appready.DefaultHealthURL(m)
		if warnOnly {
			if _, err := readiness.Probe(ctx, nil, m.Name, url); err != nil && readiness.IsUnreachable(err) {
				continue
			}
		}
		if err := pluginWait(ctx, m.Name, url, max(time.Until(deadline), time.Millisecond), readiness.WithInterval(pluginReadyEvery)); err != nil {
			if warnOnly {
				_, _ = fmt.Fprintf(stderr, "warning: %v\n", err)
				continue
			}
			return err
		}
	}
	return nil
}

func memberNames(ms []appready.Member) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Name)
	}
	return out
}

// trackCause keeps the underlying error reachable for errors.Is/As while hiding
// any exit coder inside it (a docker *exec.ExitError): E140 exits with its
// registered status 2, not with docker's.
type trackCause struct{ err error }

func (c *trackCause) Error() string { return c.err.Error() }
func (c *trackCause) Unwrap() error { return c.err }
func (c *trackCause) ExitCode() int { return 2 }

// pluginInstallTracking tracks the manifest tables of the plugins that declare
// migrations.apply: boot (readiness proved their tables exist) in Hasura, then
// verifies them with export_metadata. v1.4: does nothing, sends no request.
// Outside a project, with the stack down, or with no tables: nothing to do.
func pluginInstallTracking(cmd *cobra.Command, pluginDir string, installed []string) error {
	// compat.V15(P7-PLUG-78): plugin install does not track manifest tables -> tracks them in Hasura after readiness (E140)
	if !compat.V15() {
		return nil
	}
	members, err := appready.LoadMembers(pluginDir, officialInstalledNames(pluginDir, installed))
	if err != nil {
		return fmt.Errorf("reading the installed plugin manifests for table tracking: %w", err)
	}
	var tracked []appready.Member
	for _, m := range members {
		if m.BootMigrations && len(m.Tables) > 0 {
			tracked = append(tracked, m)
		}
	}
	plugins := pluginTablesOf(tracked)
	if len(plugins) == 0 {
		return nil
	}
	cfg, _, err := bundleProjectConfig()
	if err != nil {
		if errors.Is(err, appready.ErrNoProject) {
			return nil // outside a project there is no Hasura to track in
		}
		return trackIssue(plugins, fmt.Errorf("loading the project config: %w", err))
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	up, serr := pluginStackState(ctx, cfg.ProjectName)
	if serr != nil {
		return trackIssue(plugins, fmt.Errorf("checking whether the stack is running: %w", serr))
	}
	if !up {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "note: the stack is not running, so the tables of "+strings.Join(memberNames(tracked), ", ")+" are not tracked yet; once it is up, rerun `nself plugin install "+strings.Join(memberNames(tracked), " ")+"` (`nself start` does not track)")
		return nil
	}
	if _, err := bundleTrackTables(ctx, cfg, plugins); err != nil {
		return trackIssue(plugins, err)
	}
	raw, err := bundleExportMetadata(ctx, cfg)
	if err != nil {
		return trackIssue(plugins, fmt.Errorf("verifying tracked tables: %w", err))
	}
	set, err := appready.TrackedTables(raw)
	if err != nil {
		return trackIssue(plugins, fmt.Errorf("verifying tracked tables: %w", err))
	}
	if name, miss := appready.Missing(tracked, set); name != "" {
		return trackIssue(untrackedOf(plugins, name, miss), errors.New("hasura does not list them after the track request"))
	}
	return nil
}

// untrackedOf narrows plugins to the one plugin that still has untracked tables.
func untrackedOf(plugins []database.PluginTables, name string, miss []string) []database.PluginTables {
	for _, p := range plugins {
		if p.Plugin == name {
			return []database.PluginTables{{Plugin: name, Tables: miss}}
		}
	}
	return plugins
}

// trackIssue builds E140: it names the plugin(s), the tables that are not
// tracked and the underlying error (the plugin that failed first when known).
func trackIssue(plugins []database.PluginTables, cause error) error {
	var te *database.TrackError
	if errors.As(cause, &te) && te.Plugin != "" {
		for _, p := range plugins {
			if p.Plugin == te.Plugin {
				plugins = []database.PluginTables{p}
				break
			}
		}
	}
	var parts, names []string
	for _, p := range plugins {
		q := make([]string, 0, len(p.Tables))
		for _, t := range p.Tables {
			q = append(q, database.QualifyPluginTable(t))
		}
		names = append(names, p.Plugin)
		parts = append(parts, fmt.Sprintf("%s (%s)", p.Plugin, strings.Join(q, ", ")))
	}
	e := errs.Newf("E140", "plugin %s: tables not tracked in Hasura: %s", strings.Join(names, ", "), strings.Join(parts, "; ")).WithWhy(cause.Error())
	e.Wrapped = &trackCause{err: cause}
	return e
}
