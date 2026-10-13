package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/sysexec"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var dnsSetupCmd = &cobra.Command{
	Use:   "dns",
	Short: "Add project domains to /etc/hosts (run with sudo)",
	Long: `Add all project domains to /etc/hosts so local *.custom-domain URLs resolve.

This command reads your project's .env, collects every domain that nself
generates nginx config for, and appends missing entries to /etc/hosts.

Because /etc/hosts requires root access, run this once with sudo:

  sudo nself dns-setup

Wildcard entries (e.g. *.pro.ummat.local) cannot go in /etc/hosts. For
wildcard subdomains, install dnsmasq and add:

  address=/.pro.ummat.local/127.0.0.1

Only needed for custom BASE_DOMAIN values (e.g. ummat.local). Standard
nself.org and localhost setups do not need this.`,
	RunE: runDNSSetup,
}

func init() {
	dnsSetupCmd.Flags().BoolP("dry-run", "n", false, "Print entries that would be added without writing")
	dnsSetupCmd.Flags().StringP("project", "p", "", "Path to nself project directory (useful when running with sudo)")
	trustCmd.AddCommand(dnsSetupCmd)
}

func runDNSSetup(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	projectFlag, _ := cmd.Flags().GetString("project")

	ui.CommandHeader("nself dns-setup", "Add project domains to /etc/hosts")

	var workdir string
	if projectFlag != "" {
		// User explicitly provided a path — resolve it and validate it's an nself project.
		abs, err := filepath.Abs(projectFlag)
		if err != nil {
			return fmt.Errorf("resolving project path: %w", err)
		}
		if _, err := config.FindNSelfRoot(abs); err != nil {
			return fmt.Errorf("no nself project found at %s", abs)
		}
		workdir = abs
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		root, err := config.FindNSelfRoot(cwd)
		if err != nil {
			return fmt.Errorf("no nself project found in current directory or parents — use --project /path/to/backend or cd to your project first")
		}
		workdir = root
	}

	cfg, err := config.Load(workdir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	gen := ssl.NewGenerator(cfg)
	domains := gen.CollectDomains()

	// Filter to only the entries that can go in /etc/hosts (no wildcards, no IPs).
	// We surface wildcards separately so the user knows what needs dnsmasq.
	var wildcards []string
	var hostable []string
	for _, d := range domains {
		if strings.HasPrefix(d, "*.") || strings.Contains(d, "*") {
			wildcards = append(wildcards, d)
		} else {
			hostable = append(hostable, d)
		}
	}

	if len(hostable) == 0 && len(wildcards) == 0 {
		ui.Info("No domains to configure for " + cfg.BaseDomain)
		return nil
	}

	// Report wildcard entries separately — they need dnsmasq, not /etc/hosts.
	if len(wildcards) > 0 {
		ui.Warn("Wildcard domains require dnsmasq (cannot go in /etc/hosts):")
		for _, w := range wildcards {
			fmt.Printf("  address=/%s/127.0.0.1\n", strings.TrimPrefix(w, "*."))
		}
		fmt.Println()
	}

	if dryRun {
		fmt.Println("Would add to /etc/hosts:")
		for _, h := range hostable {
			fmt.Printf("  127.0.0.1\t%s\n", h)
		}
		return nil
	}

	// Bail out before any blocking admin-privilege operation if the context
	// has already been cancelled (e.g. test harness deadline).
	if err := cmd.Context().Err(); err != nil {
		return fmt.Errorf("dns-setup: %w", err)
	}

	added, err := ssl.AddHosts(hostable)
	if err != nil {
		if errors.Is(err, sysexec.ErrSkippedHeadless) {
			ui.Warn("Headless mode (NSELF_HEADLESS=1 or CI=1): /etc/hosts not changed. Run `nself dns-setup` on a machine with admin rights, or set NSELF_HEADLESS=0.")
			return nil
		}
		if errors.Is(err, ssl.ErrSudoRequired) {
			if runtime.GOOS == "darwin" {
				return rerunWithOsascript(cmd.Context(), workdir)
			}
			ui.Error(fmt.Sprintf("Permission denied — /etc/hosts requires root. Run: sudo nself dns-setup --project %s", workdir))
			return fmt.Errorf("permission denied writing /etc/hosts")
		}
		return fmt.Errorf("updating /etc/hosts: %w", err)
	}

	if added == 0 {
		ui.Success("All domains already in /etc/hosts — nothing to do.")
	} else {
		ui.Success(fmt.Sprintf("Added %d domain(s) to /etc/hosts", added))
	}

	if len(wildcards) > 0 {
		ui.Info("Remember to configure dnsmasq for wildcard domains (see above).")
	}

	return nil
}

// rerunWithOsascript re-executes the current binary with dns-setup via osascript,
// which presents a native macOS password dialog for administrator privileges.
// workdir is passed via --project so the elevated process resolves the same project.
//
// ctx is the cobra command context. If ctx is already done before we can show
// the dialog, the function returns a context error immediately rather than
// blocking on a native OS auth dialog. This prevents test harnesses from
// hanging when they cancel the context via a deadline.
//
// Only an interactive TTY session shows the native macOS admin dialog. In
// non-TTY contexts (tests, CI, piped output) the dialog would block
// indefinitely — return the sudo hint message instead.
func rerunWithOsascript(ctx context.Context, workdir string) error {
	// Refuse to launch a blocking OS dialog if the context is already done.
	select {
	case <-ctx.Done():
		return fmt.Errorf("dns-setup: %w", ctx.Err())
	default:
	}

	// In non-interactive contexts (CI, tests, piped output) skip the blocking
	// macOS password dialog and return a clear sudo hint instead.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		ui.Error(fmt.Sprintf("Permission denied — /etc/hosts requires root. Run: sudo nself dns-setup --project %s", workdir))
		return fmt.Errorf("permission denied writing /etc/hosts")
	}

	self, err := os.Executable()
	if err != nil {
		ui.Error(fmt.Sprintf("Permission denied — /etc/hosts requires root. Run: sudo nself dns-setup --project %s", workdir))
		return fmt.Errorf("permission denied writing /etc/hosts")
	}
	// Guard: reject shell metacharacters in workdir before embedding into the
	// AppleScript do shell script string. Backticks and $() execute arbitrary
	// shell commands with administrator privileges; ;|&<> allow chaining and
	// redirection. A legitimate file-system path never needs these characters.
	const shellMetachars = "`$();&|<>\n\r"
	if strings.ContainsAny(filepath.Clean(workdir), shellMetachars) {
		ui.Error("Permission denied — project path contains invalid characters. Run: sudo nself dns-setup --project <path>")
		return fmt.Errorf("workdir contains shell metacharacters (injection guard)")
	}

	// Never raise a native admin dialog on a headless host. dns-setup already
	// returns before this point when headless; this is the last line of defence.
	if sysexec.Headless() {
		return sysexec.ErrSkippedHeadless
	}

	ui.Info("Requesting administrator access to update /etc/hosts...")
	// Escape the workdir and binary paths to prevent AppleScript injection.
	// Backslashes and double-quotes are escaped; shell metacharacters were
	// already rejected by the guard above.
	escapedSelf := strings.ReplaceAll(filepath.Clean(self), `\`, `\\`)
	escapedSelf = strings.ReplaceAll(escapedSelf, `"`, `\"`)
	escapedWorkdir := strings.ReplaceAll(filepath.Clean(workdir), `\`, `\\`)
	escapedWorkdir = strings.ReplaceAll(escapedWorkdir, `"`, `\"`)
	script := fmt.Sprintf(`do shell script "%s dns-setup --project %s" with administrator privileges`, escapedSelf, escapedWorkdir)
	out, err := sysexec.Output(ctx, "osascript", "-e", script)
	if err != nil {
		ui.Error(fmt.Sprintf("Permission denied — /etc/hosts requires root. Run: sudo nself dns-setup --project %s", workdir))
		return fmt.Errorf("permission denied writing /etc/hosts")
	}
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	return nil
}
