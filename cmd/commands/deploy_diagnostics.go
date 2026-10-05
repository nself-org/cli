package commands

// Purpose: RunE implementations for "nself deploy logs", "health", and
// "check-access". Inputs are the cobra command/args; outputs are printed
// diagnostics or an error.
// Constraints: split out of deploy.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runDeployLogs(cmd *cobra.Command, args []string) error {
	workdir, err := projectRoot()
	if err != nil {
		return err
	}

	serverFilter, _ := cmd.Flags().GetString("server")
	if serverFilter != "" {
		// T06: Stream logs from a specific remote server via SSH.
		inv, loadErr := controlplane.Load(workdir)
		if loadErr != nil {
			return fmt.Errorf("deploy logs: load inventory: %w", loadErr)
		}
		for _, env := range inv.Environments {
			for _, srv := range env.Servers {
				if srv.Name != serverFilter {
					continue
				}
				if srv.Host == "" {
					// Local server: fall through to docker compose logs below.
					break
				}
				keyPath := os.Getenv(srv.SSHKeyRef)
				remotePath := srv.RemotePath
				if remotePath == "" {
					remotePath = "/opt/nself"
				}
				logsCmd := fmt.Sprintf("cd %s && docker compose logs --tail=200 -f", remotePath)
				sshTarget := srv.Host
				sc := exec.CommandContext(cmd.Context(), "ssh",
					"-i", keyPath,
					"-o", "BatchMode=yes",
					"-o", "ForwardAgent=no",
					"-o", "StrictHostKeyChecking=accept-new",
					sshTarget, logsCmd)
				sc.Stdout = os.Stdout
				sc.Stderr = os.Stderr
				return sc.Run()
			}
		}
		return fmt.Errorf("deploy logs: --server %q not found in inventory", serverFilter)
	}

	// Default: local docker compose logs.
	c := exec.CommandContext(cmd.Context(), "docker", "compose", "logs", "--tail=200")
	c.Dir = workdir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func runDeployHealth(cmd *cobra.Command, args []string) error {
	workdir, err := projectRoot()
	if err != nil {
		return err
	}

	serverFilter, _ := cmd.Flags().GetString("server")
	jsonOut, _ := cmd.Flags().GetBool("json")

	if serverFilter != "" {
		return runDeployHealthOnServer(cmd, workdir, serverFilter, jsonOut)
	}

	// Gap #12: "nself deploy health <target>" (positional arg, no --server)
	// previously ignored the target entirely and always ran local doctor —
	// silently misleading for staging/prod. Resolve the target the same way
	// runDeploy does and probe its primary server remotely when the target
	// names a configured remote environment. "local" (or no target) keeps
	// the original local-doctor behavior unchanged (back-compat).
	if len(args) == 1 {
		target, resolveErr := resolveTarget(args[0])
		if resolveErr != nil {
			return resolveErr
		}
		if target != "local" {
			inv, loadErr := controlplane.Load(workdir)
			if loadErr != nil {
				return fmt.Errorf("deploy health: load inventory: %w", loadErr)
			}
			if env, ok := inv.Environments[target]; ok {
				if srv, found := findDBTargetServer(env, ""); found && srv.Host != "" {
					return runDeployHealthOnServer(cmd, workdir, srv.Name, jsonOut)
				}
			}
			// No remote host configured for this target (e.g. only
			// NSELF_DEPLOY_HOST_<TARGET> style legacy env vars, no inventory
			// entry) — tell the user explicitly rather than silently running
			// local checks against the wrong environment's name.
			if host, ok := ResolveLegacyDeployHost(target); ok {
				return runDeployHealthOverSSH(cmd, host, jsonOut)
			}
			return fmt.Errorf("deploy health: no server configured for target %q (set NSELF_DEPLOY_HOST_%s or add it to .nself/control-plane.yaml)", target, strings.ToUpper(target))
		}
	}

	return runCLISelf(cmd.Context(), workdir, "doctor")
}

func runDeployCheckAccess(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")

	// T07: Re-point to controlplane probe path when inventory is present.
	root, rootErr := projectRoot()
	if rootErr == nil {
		cpYaml := filepath.Join(root, ".nself", "control-plane.yaml")
		if _, err := os.Stat(cpYaml); err == nil {
			inv, loadErr := controlplane.Load(root)
			if loadErr == nil {
				prober := controlplane.NewSSHProber(root, false)
				statuses := controlplane.Resolve(inv, prober)

				if !jsonOut {
					ui.Warn("'nself deploy check-access' is deprecated. Use 'nself env target probe' for full per-server capability details.")
					fmt.Println()
				}

				type accessRow struct {
					Env        string `json:"env"`
					Server     string `json:"server"`
					Capability string `json:"capability"`
					Reason     string `json:"reason,omitempty"`
				}
				var rows []accessRow
				allOK := true
				for _, ts := range statuses {
					if ts.Capability == controlplane.CapHidden {
						continue
					}
					rows = append(rows, accessRow{
						Env:        ts.Env,
						Server:     ts.Server,
						Capability: string(ts.Capability),
						Reason:     ts.Reason,
					})
					if ts.Capability != controlplane.CapManage {
						allOK = false
					}
				}

				if jsonOut {
					b, _ := json.MarshalIndent(map[string]interface{}{"servers": rows, "all_ok": allOK}, "", "  ")
					fmt.Println(string(b))
					return nil
				}

				for _, row := range rows {
					if row.Capability == string(controlplane.CapManage) {
						ui.Success(fmt.Sprintf("%s/%s: %s", row.Env, row.Server, row.Capability))
					} else {
						ui.Warn(fmt.Sprintf("%s/%s: %s — %s", row.Env, row.Server, row.Capability, row.Reason))
					}
				}
				if allOK {
					ui.Success("All deploy targets reachable")
				}
				return nil
			}
		}
	}

	// Legacy fallback: shallow env-var check (back-compat for no-yaml installs).
	ok := true
	for _, name := range legacyHostVarNames() {
		v := os.Getenv(name)
		if v == "" {
			ui.Warn(fmt.Sprintf("%s is not set (deploy to this target will run locally)", name))
			ok = false
			continue
		}
		ui.Success(fmt.Sprintf("%s = %s", name, v))
	}
	if !ok {
		return nil
	}
	ui.Success("All deploy targets reachable")
	return nil
}

// ── output helpers ───────────────────────────────────────────────────────────

// legacyHostVarNames lists the NSELF_DEPLOY_HOST_<ENV> variables to check:
// staging then prod (always reported, set or not), then every other
// environment the process has a host variable for, sorted.
func legacyHostVarNames() []string {
	const prefix = "NSELF_DEPLOY_HOST_"
	names := []string{prefix + "STAGING", prefix + "PROD"}
	var extra []string
	for _, kv := range os.Environ() {
		k, v, found := strings.Cut(kv, "=")
		if found && strings.HasPrefix(k, prefix) && len(k) > len(prefix) && v != "" && k != names[0] && k != names[1] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	return append(names, extra...)
}
