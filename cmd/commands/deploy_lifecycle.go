package commands

// Purpose: RunE implementations for deploy status, rollback, and promote.
// Constraints: split out of deploy.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/deploy/bluegreen"
	"github.com/nself-org/cli/internal/promote"
	"github.com/nself-org/cli/internal/ui"
	"github.com/nself-org/cli/sdk/go/v2/remote"

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
		if handled, rbErr := rollbackPromoteForTarget(cmd.Context(), workdir, target); handled {
			return rbErr
		}
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
	// An empty tag reads the latest promote record and restores its snapshot.
	// This ensures rollback always targets
	// the last honest production-change surface (nself promote), not an
	// arbitrary git tag or manual state.
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

// rollbackRemoteV15 undoes the last v1.5 remote apply recorded for target on every
// host that kept a pre-deploy tar, restoring the database backup on backup_host
// only. handled=false (no such record) keeps the old rollback. A record already
// rolled back is refused (the database would be restored twice).
func rollbackRemoteV15(cmd *cobra.Command, workdir, target string) (bool, error) {
	st, err := deploy.LoadDeployState(workdir)
	if err != nil && target != "local" { // unreadable or corrupt: never fall back blind (D-0144)
		return true, fmt.Errorf("deploy rollback: %w; nothing was changed (fix or remove %s, then run it again)", err, deploy.DeployStateFile)
	}
	tars := st.PreDeployTars
	if target == "local" || len(tars) == 0 || st.Target != target {
		return false, nil
	}
	if st.RolledBack {
		return true, fmt.Errorf("deploy rollback: the last deploy of %s was already rolled back; nothing was changed (deploy again to get a new rollback point)", target)
	}
	ctx, stop := deploy.WithInterrupt(cmd.Context())
	defer stop()
	inv, _ := controlplane.Load(workdir)
	hosts := make([]string, 0, len(tars))
	for h := range tars {
		hosts = append(hosts, h)
	}
	sort.Slice(hosts, func(i, j int) bool { // the database host first, then by name
		if a, b := hosts[i] == st.BackupHost, hosts[j] == st.BackupHost; a != b {
			return a
		}
		return hosts[i] < hosts[j]
	})
	for _, host := range hosts { // every record is checked before any host is opened
		if len(st.PreDeployChanged[host]) == 0 {
			return true, fmt.Errorf("deploy rollback: the record for %s has no list of changed paths; it cannot be rolled back safely (nothing was changed)", host)
		}
		if strings.Index(tars[host], "/"+deploy.BackupDir+"/pre-deploy-") <= 0 {
			return true, fmt.Errorf("deploy rollback: the recorded tar %q for %s is not a pre-deploy tar; nothing was changed", tars[host], host)
		}
	}
	manualDB := st.BackupID != "" && tars[st.BackupHost] == ""
	if manualDB {
		fmt.Printf("  [warn] database backup %s was not restored: %s kept no changes of this deploy. If the database must go back, run on %s: nself backup restore %s --yes\n", st.BackupID, st.BackupHost, st.BackupHost, st.BackupID)
	}
	var failed []string
	for _, host := range hosts {
		tar := tars[host]
		idx := strings.Index(tar, "/"+deploy.BackupDir+"/pre-deploy-")
		srv, key := controlplane.Server{Name: target + "-app", Host: host, RemotePath: tar[:idx]}, sshKeyPath()
		if inv != nil {
			for _, e := range inv.Environments {
				for _, c := range e.Servers {
					if spec, err := remote.ParseHostSpec(c.Host); err == nil && spec.String() == host {
						srv.Name, srv.Host = c.Name, c.Host
						if v := os.Getenv(c.SSHKeyRef); v != "" {
							key = v
						}
					}
				}
			}
		}
		r, err := openDeployRemote(ctx, srv, key, target)
		if err == nil {
			backupID := ""
			if host == st.BackupHost {
				backupID = st.BackupID
			}
			err = deploy.RollbackRemote(ctx, deploy.RollbackOptions{
				Remote: r, RemotePath: srv.RemotePath, Tar: tar, BackupID: backupID,
				Changed: st.PreDeployChanged[host], Absent: st.PreDeployAbsent[host],
				DBRestored: st.DBRestoredAt != "", StrictHasura: hasuraStrictFor(workdir, target),
				OnDBRestored: func() error {
					st.DBRestoredAt = time.Now().UTC().Format(time.RFC3339)
					return deploy.SaveDeployState(workdir, st)
				},
				Clock: deployGateClock, Out: os.Stdout,
			})
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", host, err))
		}
	}
	if len(failed) > 0 {
		return true, fmt.Errorf("deploy rollback failed on %d host(s): %s", len(failed), strings.Join(failed, "; "))
	}
	st.RolledBack = true
	if !manualDB {
		st.BackupID = ""
	}
	_ = deploy.SaveDeployState(workdir, st)
	return true, nil
}
