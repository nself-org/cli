package commands

// Purpose: implements the "nself deploy environments" subcommand, which lists
// the configured deploy targets and their environment metadata. Inputs come
// from the project's control-plane config; output is a table or JSON row set.
// Constraints: split out of deploy.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/output"

	"github.com/spf13/cobra"
)

// envServerRow is the stable JSON schema for one server entry returned by
// "nself deploy environments". No secrets: SSHKeyRef value is never emitted.
type envServerRow struct {
	Name       string `json:"name"`
	Role       string `json:"role"`
	Capability string `json:"capability"`
	Reason     string `json:"reason,omitempty"`
}

// envEnvironmentRow is the stable JSON schema for one environment entry.
type envEnvironmentRow struct {
	Name    string         `json:"name"`
	Kind    string         `json:"kind"`
	Servers []envServerRow `json:"servers"`
}

// deployEnvironmentsOutput is the top-level JSON response consumed by Admin.
type deployEnvironmentsOutput struct {
	Environments []envEnvironmentRow `json:"environments"`
}

// runDeployEnvironments resolves the inventory and capability of every server,
// then emits the Admin-contract JSON.  No secret values are included.
func runDeployEnvironments(cmd *cobra.Command, _ []string) error {
	root, err := projectRoot()
	if err != nil {
		return err
	}

	inv, err := controlplane.Load(root)
	if err != nil {
		return fmt.Errorf("deploy environments: %w", err)
	}

	prober := controlplane.NewSSHProber(root, false)
	statuses := controlplane.Resolve(inv, prober)

	// Build a fast lookup: env/server → TargetStatus.
	type envServerKey struct{ env, server string }
	lookup := make(map[envServerKey]controlplane.TargetStatus, len(statuses))
	for _, ts := range statuses {
		lookup[envServerKey{ts.Env, ts.Server}] = ts
	}

	// Sort environments: local first, then alphabetically.
	envNames := make([]string, 0, len(inv.Environments))
	for name := range inv.Environments {
		envNames = append(envNames, name)
	}
	sort.Slice(envNames, func(i, j int) bool {
		if envNames[i] == "local" {
			return true
		}
		if envNames[j] == "local" {
			return false
		}
		return envNames[i] < envNames[j]
	})

	out := deployEnvironmentsOutput{
		Environments: make([]envEnvironmentRow, 0, len(envNames)),
	}
	for _, envName := range envNames {
		env := inv.Environments[envName]
		row := envEnvironmentRow{
			Name:    env.Name,
			Kind:    env.Kind,
			Servers: make([]envServerRow, 0, len(env.Servers)),
		}
		for _, srv := range env.Servers {
			ts := lookup[envServerKey{envName, srv.Name}]
			cap := string(ts.Capability)
			if cap == "" {
				cap = string(controlplane.CapHidden)
			}
			row.Servers = append(row.Servers, envServerRow{
				Name:       srv.Name,
				Role:       string(srv.Role),
				Capability: cap,
				Reason:     ts.Reason,
			})
		}
		out.Environments = append(out.Environments, row)
	}

	if jsonEnvelopeOn(cmd) {
		return output.EmitData(pilotWriter(), "deploy environments", out)
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("deploy environments: marshal: %w", err)
	}
	fmt.Println(string(b))
	return nil
}

// ── runDeploy ────────────────────────────────────────────────────────────────
