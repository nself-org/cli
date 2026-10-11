package commands

import (
	"fmt"
	"runtime"

	"github.com/nself-org/cli/internal/trust"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// trustOpts holds resolved options for the trust command.
type trustOpts struct {
	SkipDNS   bool
	SkipSSL   bool
	SkipPorts bool
	Status    bool
	Undo      bool
	Project   string
}

var trustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Set up local dev trust (DNS, SSL, port forwarding)",
	Long: `Set up trusted local development: dnsmasq DNS resolution, mkcert SSL certificates,
and port forwarding so all *.local projects work with HTTPS.

Covers ALL .local projects with a single setup — no per-project configuration needed.

Run once per machine:
  nself trust

Check current status:
  nself trust --status

Undo all changes:
  nself trust --undo`,
	RunE: runTrust,
}

// trustStatusCmd is the `nself trust status` subcommand.
var trustStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show trusted cert CAs, last rotation date, and expiry warnings",
	Long: `Show the current state of all local dev trust components.

Lists trusted certificate authorities, SSL certificate expiry dates, and warns
when any certificate expires within 30 days.

Exit codes:
  0 — all trust components configured and no expiry warnings
  2 — one or more components missing or cert expiry <30 days`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		opts := trustOpts{Project: project}
		cfg := buildTrustConfig(opts)
		return runTrustStatusDetailed(cmd, cfg)
	},
}

func init() {
	trustCmd.Flags().Bool("skip-dns", false, "Skip dnsmasq and /etc/resolver setup")
	trustCmd.Flags().Bool("skip-ssl", false, "Skip mkcert CA and certificate generation")
	trustCmd.Flags().Bool("skip-ports", false, "Skip port forwarding setup")
	trustCmd.Flags().Bool("status", false, "Show current trust status and exit")
	trustCmd.Flags().Bool("undo", false, "Print instructions to undo all trust changes")
	trustCmd.Flags().StringP("project", "p", "", "Path to nself project directory (allows running from any cwd)")

	trustStatusCmd.Flags().StringP("project", "p", "", "Path to nself project directory")

	trustCmd.AddCommand(trustStatusCmd)
	RootCmd.AddCommand(trustCmd)
}

// resolveTrustOpts reads flag values from the cobra command into a trustOpts struct.
func resolveTrustOpts(cmd *cobra.Command) trustOpts {
	skipDNS, _ := cmd.Flags().GetBool("skip-dns")
	skipSSL, _ := cmd.Flags().GetBool("skip-ssl")
	skipPorts, _ := cmd.Flags().GetBool("skip-ports")
	status, _ := cmd.Flags().GetBool("status")
	undo, _ := cmd.Flags().GetBool("undo")
	project, _ := cmd.Flags().GetString("project")
	return trustOpts{
		SkipDNS:   skipDNS,
		SkipSSL:   skipSSL,
		SkipPorts: skipPorts,
		Status:    status,
		Undo:      undo,
		Project:   project,
	}
}

func runTrust(cmd *cobra.Command, args []string) error {
	opts := resolveTrustOpts(cmd)

	ui.CommandHeader("nself trust", "Local dev trust setup")

	// Validate OS support.
	goos := runtime.GOOS
	if goos != "darwin" && goos != "linux" {
		ui.Warn(fmt.Sprintf("nself trust is not supported on %s (supports macOS and Linux)", goos))
		if jsonEnvelopeOn(cmd) {
			setData(cmd, dataResult{OK: false}) // nothing was set up
		}
		return nil
	}

	// Build TrustConfig — try to load project config but fall back to defaults.
	cfg := buildTrustConfig(opts)

	// --status: show status table and exit.
	if opts.Status {
		return runTrustStatus(cfg)
	}

	// --undo: print instructions and exit.
	if opts.Undo {
		return runTrustUndo(cfg)
	}

	// Print what will be done.
	printTrustPlan(opts, goos)

	// Execute trust setup.
	result, err := trust.Setup(cfg)
	if err != nil {
		ui.Error(fmt.Sprintf("Trust setup failed: %v", err))
		return err
	}

	// Print summary.
	printTrustSummary(result)

	if jsonEnvelopeOn(cmd) && len(result.Errors) > 0 {
		setData(cmd, dataResult{OK: false}) // some steps reported errors
	}

	// Surface any non-fatal errors.
	if len(result.Errors) > 0 {
		fmt.Println()
		ui.Warn("Some steps reported errors:")
		for _, e := range result.Errors {
			fmt.Printf("  - %v\n", e)
		}
	}

	return nil
}
