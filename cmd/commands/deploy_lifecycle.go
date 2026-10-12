package commands

// Purpose: RunE implementations for deploy status, rollback, and promote.
// Constraints: split out of deploy.go (CLI-R12) as a pure move, no behavior change.
//              rollbackRemoteV15 lives in deploy_lifecycle_rollback.go (size ratchet).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy/bluegreen"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/promote"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runDeployStatus(cmd *cobra.Command, args []string) error {
	env, _ := cmd.Flags().GetString("env")
	jsonOut, _ := cmd.Flags().GetBool("json")
	serverFilter, _ := cmd.Flags().GetString("server")

	// Determine base state from docker ps (local stack indicator).
	state := "unknown"
	if env == "" {
		state = "no-target"
	} else if _, err := resolveTarget(env); err != nil {
		return err
	}
	if _, err := exec.LookPath("docker"); err == nil {
		out, derr := exec.CommandContext(cmd.Context(), "docker", "ps", "--format", "{{.Names}}").Output()
		if derr == nil && strings.Contains(string(out), "postgres") {
			state = "running"
		} else if state != "no-target" {
			state = "not-running"
		}
	}

	// T06: If control-plane inventory exists, enrich with per-server capability.
	root, rootErr := projectRoot()
	if rootErr == nil {
		cpYaml := filepath.Join(root, ".nself", "control-plane.yaml")
		if _, err := os.Stat(cpYaml); err == nil {
			inv, loadErr := controlplane.Load(root)
			if loadErr == nil {
				prober := controlplane.NewSSHProber(root, false)
				statuses := controlplane.Resolve(inv, prober)

				var rows []deployStatusServer
				for _, ts := range statuses {
					if ts.Capability == controlplane.CapHidden {
						continue
					}
					if serverFilter != "" && ts.Server != serverFilter {
						continue
					}
					// Determine role from inventory.
					role := ""
					if envDef, ok := inv.Environments[ts.Env]; ok {
						for _, srv := range envDef.Servers {
							if srv.Name == ts.Server {
								role = string(srv.Role)
								break
							}
						}
					}
					rows = append(rows, deployStatusServer{
						Env:        ts.Env,
						Server:     ts.Server,
						Role:       role,
						Capability: string(ts.Capability),
						Reason:     ts.Reason,
						LatencyMS:  ts.LatencyMS,
					})
				}

				if jsonOut {
					legacy := map[string]interface{}{"target": env, "state": state, "servers": rows}
					return emitDeployDoc("deploy status", legacy, newDeployStatusData(env, state, rows), nil)
				}
				fmt.Printf("target=%s state=%s\n", env, state)
				for _, row := range rows {
					fmt.Printf("  server=%s/%s role=%-15s capability=%s", row.Env, row.Server, row.Role, row.Capability)
					if row.LatencyMS > 0 {
						fmt.Printf(" latency=%dms", row.LatencyMS)
					}
					if row.Reason != "" {
						fmt.Printf(" reason=%q", row.Reason)
					}
					fmt.Println()
				}
				return nil
			}
		}
	}

	// Fallback: legacy single-host output.
	status := map[string]string{
		"target": env,
		"state":  state,
	}
	if jsonOut {
		return emitDeployDoc("deploy status", status, newDeployStatusData(env, state, nil), nil)
	}
	fmt.Printf("target=%s state=%s\n", status["target"], status["state"])
	return nil
}

func runDeployRollback(cmd *cobra.Command, args []string) error {
	target := "local"
	if len(args) == 1 {
		t, err := resolveTarget(args[0])
		if err != nil {
			return err
		}
		target = t
	}

	workdir, err := projectRoot()
	if err != nil {
		return err
	}

	bgEnabled := os.Getenv("NSELF_FEATURE_BLUE_GREEN_DEPLOY") == "true"
	if bgEnabled {
		ui.Info(fmt.Sprintf("Rolling back blue/green deploy for target: %s", target))
		cfg := bluegreen.DeployConfig{ProjectRoot: workdir}
		result := bluegreen.Rollback(cmd.Context(), cfg)
		if !result.Success {
			return fmt.Errorf("blue/green rollback failed: %s", result.Error)
		}
		ui.Success(fmt.Sprintf("Blue/green rollback complete in %s — all traffic restored to blue", result.Duration.Round(time.Millisecond)))
		return nil
	}

	// compat.V15(P7-DEPL-24): promote snapshot rollback -> restore the remote pre-deploy tar (and the prod database backup) in place
	if compat.V15() {
		// compat.V15(P7-DEPL-15): local promote snapshot -> latest verified remote promotion record
		if handled, rbErr := rollbackPromoteForTarget(withPromoteRollbackConfirmation(cmd.Context(), cmd), workdir, target); handled {
			return rbErr
		}
		if handled, rbErr := rollbackRemoteV15(cmd, workdir, target); handled {
			return rbErr
		}
		return errs.New("E485", "no usable v1.5 rollback record; nothing was changed")
	}

	// PREVIEW: rollback is a PREVIEW feature. It restores the last promote snapshot
	// created by nself promote. DNS failback and full cluster-level rollback are not
	// yet automated — see cross-cutting.md §3 Lifecycle for the roadmap.
	ui.Warn("nself deploy rollback is a PREVIEW feature. It restores the last pre-promote backup snapshot. No DNS changes are made automatically.")
	ui.Info(fmt.Sprintf("Rolling back last deployment for target: %s", target))

	// DEP-04: wire to last promote tag written by nself promote.
	// An empty tag reads the latest promote record and restores its snapshot.
	if err := promote.Rollback(cmd.Context(), workdir, ""); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	ui.Success(fmt.Sprintf("Rollback for %s completed — prior promote state restored", target))
	return nil
}

// runDeployPromote flips Nginx to 100% green after a manual canary review.
func runDeployCanaryPromote(cmd *cobra.Command, args []string) error {
	workdir, err := projectRoot()
	if err != nil {
		return err
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")

	ui.Info("Promoting canary to 100% green traffic...")
	cfg := bluegreen.DeployConfig{
		ProjectRoot: workdir,
		DryRun:      dryRun,
	}
	result := bluegreen.Promote(cmd.Context(), cfg)
	if !result.Success {
		return fmt.Errorf("promote failed: %s", result.Error)
	}
	ui.Success(fmt.Sprintf("Promoted to 100%% green in %s", result.Duration.Round(time.Millisecond)))
	return nil
}
