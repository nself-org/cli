package commands

// Purpose: after `nself plugin install` of a plugin that declares
// migrations.apply: boot, wait for its migrations to be applied (P7-PLUG-19,
// contract:plugin.boot-migrations). The plugin applies its own SQL at boot; the
// CLI only waits on /health.
// Inputs: the plugin directory and the installed official plugin names.
// Outputs: nil, or the readiness error (E118/E119, exit 2) in v1.5; in v1.4 the
// same error is printed as a warning and nil is returned.
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
	"github.com/nself-org/cli/internal/docker"
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
)

// stackRunning reports whether the project has a running container.
func stackRunning(ctx context.Context, projectName string) bool {
	svcs, err := docker.RunningConfigHashes(ctx, projectName)
	if err != nil {
		return false
	}
	for _, s := range svcs {
		if s.State == "running" {
			return true
		}
	}
	return false
}

// pluginInstallReadiness waits for boot migrations of the installed plugins.
func pluginInstallReadiness(cmd *cobra.Command, pluginDir string, installed []string) error {
	var names []string
	for _, n := range installed {
		if plugin.IsThirdPartyInstallSource(n) {
			continue
		}
		// A name with no directory is not installed here: nothing to wait on.
		// Anything else that cannot be read is a real problem (below).
		if _, err := os.Stat(filepath.Join(pluginDir, n)); err == nil {
			names = append(names, n)
		}
	}
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
