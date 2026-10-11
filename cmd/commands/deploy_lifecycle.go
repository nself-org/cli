package commands

// Purpose: RunE implementations for "nself deploy status", "rollback", and
// "promote". Inputs are the cobra command/args; outputs are printed status or
// promotion results, or an error.
// Constraints: split out of deploy.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/deploy/bluegreen"
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

				type serverStatus struct {
					Env        string `json:"env"`
					Server     string `json:"server"`
					Role       string `json:"role"`
					Capability string `json:"capability"`
					Reason     string `json:"reason,omitempty"`
					LatencyMS  int    `json:"latency_ms,omitempty"`
				}
				var rows []serverStatus
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
					rows = append(rows, serverStatus{
						Env:        ts.Env,
						Server:     ts.Server,
						Role:       role,
						Capability: string(ts.Capability),
						Reason:     ts.Reason,
						LatencyMS:  ts.LatencyMS,
					})
				}

				if jsonOut {
					b, _ := json.MarshalIndent(map[string]interface{}{
						"target":  env,
						"state":   state,
						"servers": rows,
					}, "", "  ")
					fmt.Println(string(b))
					return nil
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
		b, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(b))
		return nil
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

	// Blue/green rollback path (Y17).
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
		if handled, rbErr := rollbackRemoteV15(cmd, workdir, target); handled {
			return rbErr
		}
	}

	// PREVIEW: rollback is a PREVIEW feature. It restores the last promote snapshot
	// created by nself promote. DNS failback and full cluster-level rollback are not
	// yet automated — see cross-cutting.md §3 Lifecycle for the roadmap.
	ui.Warn("nself deploy rollback is a PREVIEW feature. It restores the last pre-promote backup snapshot. No DNS changes are made automatically.")
	ui.Info(fmt.Sprintf("Rolling back last deployment for target: %s", target))

	// DEP-04: wire to last promote tag written by nself promote.
	// promote.Rollback with an empty tag reads the latest promote record from
	// <projectDir>/.nself/promotions/ and restores the backup snapshot created
	// before that promotion was applied. This ensures rollback always targets
	// the last honest production-change surface (nself promote), not an
	// arbitrary git tag or manual state.
	if err := promote.Rollback(cmd.Context(), workdir, ""); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	ui.Success(fmt.Sprintf("Rollback for %s completed — prior promote state restored", target))
	return nil
}

// runDeployPromote flips Nginx to 100% green after a manual canary review.
func runDeployPromote(cmd *cobra.Command, args []string) error {
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

// rollbackRemoteV15 undoes the last v1.5 remote apply recorded for target. It
// reports handled=false when there is no such record, so the caller keeps the
// old rollback.
func rollbackRemoteV15(cmd *cobra.Command, workdir, target string) (bool, error) {
	st, _ := deploy.LoadDeployState(workdir)
	if target == "local" || st.PreDeployTar == "" || st.Target != target {
		return false, nil
	}
	idx := strings.Index(st.PreDeployTar, "/"+deploy.BackupDir+"/pre-deploy-")
	if idx <= 0 {
		return true, fmt.Errorf("deploy rollback: the recorded tar %q is not a pre-deploy tar; nothing was changed", st.PreDeployTar)
	}
	rp := st.PreDeployTar[:idx]
	srv, key := controlplane.Server{Name: target + "-app", Host: st.BackupHost, RemotePath: rp}, sshKeyPath()
	if inv, err := controlplane.Load(workdir); err == nil {
		for _, e := range inv.Environments {
			for _, c := range e.Servers {
				if c.Host == st.BackupHost {
					srv.Name = c.Name
					if v := os.Getenv(c.SSHKeyRef); v != "" {
						key = v
					}
				}
			}
		}
	}
	r, err := openDeployRemote(cmd.Context(), srv, key, target)
	if err != nil {
		return true, err
	}
	err = deploy.RollbackRemote(cmd.Context(), deploy.RollbackOptions{Remote: r, RemotePath: srv.RemotePath, Tar: st.PreDeployTar, BackupID: st.BackupID, Clock: deployGateClock, Out: os.Stdout})
	if err == nil {
		st.RolledBack = true
		_ = deploy.SaveDeployState(workdir, st)
	}
	return true, err
}
