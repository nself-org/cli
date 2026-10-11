package commands

// Purpose: `nself env target probe/migrate` RunE handlers plus their
// shared helpers, split out of env_target.go (CLI-R12 Batch B mechanical
// file-size split). Probe runs SSH+Docker capability checks; migrate
// synthesizes/rewrites .nself/control-plane.yaml from legacy env vars.
// Inputs: cobra command flags/args (--json, --refresh, optional
// "env[:server]" filter for probe; no args for migrate).
// Outputs: a capability matrix (table or JSON) for probe, or a migration
// summary for migrate; errors wrap controlplane package failures.
// Constraints: pure move, no behavior change. Per the security invariants
// documented in env_target.go's file header, rejectInlineSecret guards
// --host/--key-ref input and probe never forwards credentials.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/spf13/cobra"
)

// probeRow is one row of the probe matrix, safe for JSON output (no secrets).
type probeRow struct {
	Env          string `json:"env"`
	Server       string `json:"server"`
	Capability   string `json:"capability"`
	SSHReachable bool   `json:"ssh_reachable"`
	DockerOK     bool   `json:"docker_ok"`
	LatencyMS    int    `json:"latency_ms"`
	Reason       string `json:"reason,omitempty"`
}

func runEnvTargetProbe(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	refresh, _ := cmd.Flags().GetBool("refresh")

	root, err := projectRoot()
	if err != nil {
		return err
	}
	inv, err := controlplane.Load(root)
	if err != nil {
		return fmt.Errorf("env target probe: %w", err)
	}

	// Parse optional "env" or "env:server" filter.
	filterEnv, filterServer := "", ""
	if len(args) == 1 {
		parts := strings.SplitN(args[0], ":", 2)
		filterEnv = parts[0]
		if len(parts) == 2 {
			filterServer = parts[1]
		}
	}

	// Build a scoped inventory when a filter is applied.
	if filterEnv != "" {
		if _, ok := inv.Environments[filterEnv]; !ok {
			return fmt.Errorf("environment %q not found in inventory", filterEnv)
		}
		env := inv.Environments[filterEnv]
		if filterServer != "" {
			var srvs []controlplane.Server
			for _, s := range env.Servers {
				if s.Name == filterServer {
					srvs = append(srvs, s)
				}
			}
			if len(srvs) == 0 {
				return fmt.Errorf("server %q not found in environment %q", filterServer, filterEnv)
			}
			env.Servers = srvs
		}
		inv.Environments = map[string]controlplane.Environment{filterEnv: env}
	}

	prober := controlplane.NewSSHProber(root, refresh)
	statuses := controlplane.Resolve(inv, prober)

	// Build output rows — no secret values in output.
	var rows []probeRow
	for _, ts := range statuses {
		if ts.Capability == controlplane.CapHidden {
			continue // omit hidden servers from default probe output
		}
		rows = append(rows, probeRow{
			Env:          ts.Env,
			Server:       ts.Server,
			Capability:   string(ts.Capability),
			SSHReachable: ts.SSHReachable,
			DockerOK:     ts.DockerOK,
			LatencyMS:    ts.LatencyMS,
			Reason:       ts.Reason,
		})
	}

	if jsonOut {
		if jsonEnvelopeOn(cmd) {
			return emitEnv(cmd, legacyList(rows), true)
		}
		b, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	if len(rows) == 0 {
		fmt.Println("No servers to probe.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ENV\tSERVER\tCAPABILITY\tSSH\tDOCKER\tLATENCY\tREASON")
	for _, r := range rows {
		ssh := boolMark(r.SSHReachable)
		docker := boolMark(r.DockerOK)
		latency := ""
		if r.LatencyMS > 0 {
			latency = fmt.Sprintf("%dms", r.LatencyMS)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Env, r.Server, r.Capability, ssh, docker, latency, r.Reason)
	}
	return tw.Flush()
}

func runEnvTargetMigrate(_ *cobra.Command, _ []string) error {
	root, err := projectRoot()
	if err != nil {
		return err
	}

	// Load synthesizes from env vars when control-plane.yaml is absent;
	// when it exists it applies Migrate() in-place (idempotent).
	inv, err := controlplane.Load(root)
	if err != nil {
		return fmt.Errorf("env target migrate: %w", err)
	}

	if err := controlplane.Write(root, inv); err != nil {
		return fmt.Errorf("env target migrate: %w", err)
	}

	path := root + "/.nself/control-plane.yaml"
	fmt.Printf("Wrote %s (mode 0600).\n", path)
	fmt.Printf("Environments: %d\n", len(inv.Environments))
	for envName, env := range inv.Environments {
		fmt.Printf("  %s (%s): %d server(s)\n", envName, env.Kind, len(env.Servers))
	}
	fmt.Println("\nVerify with: nself env target list")
	return nil
}

// rejectInlineSecret returns an error when the value for flagName looks like
// inline key material (contains whitespace, BEGIN/END PEM markers, or path
// separators where an env-var name is expected).
//
// This guard applies specifically to --key-ref, which should be an env-var
// NAME (e.g. "NSELF_SSH_KEY_STAGING"), not a file path or base64 blob.
func rejectInlineSecret(flagName, value string) error {
	if value == "" {
		return nil
	}
	// An env-var name is all uppercase letters, digits, and underscores.
	// If the flag value looks like a file path, PEM block, or raw secret, reject it.
	if flagName == "--key-ref" {
		// Reject if it looks like a path (contains /) or a PEM header.
		if strings.Contains(value, "/") {
			return fmt.Errorf("%s must be an environment variable NAME (e.g. NSELF_SSH_KEY_STAGING), not a file path (got %q)", flagName, value)
		}
		if strings.Contains(value, "BEGIN") || strings.Contains(value, "ssh-") {
			return fmt.Errorf("%s must be an environment variable NAME, not inline key material", flagName)
		}
	}
	if flagName == "--host" {
		// Reject if it contains newlines (multi-line = PEM block) or obvious secret patterns.
		if strings.ContainsAny(value, "\n\r") {
			return fmt.Errorf("%s must be user@host form; multi-line input rejected", flagName)
		}
	}
	return nil
}

// sortedEnvNames returns the environment names from inv in deterministic order
// (local first, then alphabetical).
func sortedEnvNames(inv *controlplane.Inventory) []string {
	names := make([]string, 0, len(inv.Environments))
	for name := range inv.Environments {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		// "local" always sorts first.
		if names[i] == "local" {
			return true
		}
		if names[j] == "local" {
			return false
		}
		return names[i] < names[j]
	})
	return names
}

// boolMark returns a short human-readable indicator for a boolean probe result.
func boolMark(v bool) string {
	if v {
		return "ok"
	}
	return "-"
}
