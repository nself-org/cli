package commands

// Purpose: share a read-only host .env diagnostic between preview and doctor.
// Inputs: selected environment, inventory and optional server selector.
// Outputs: key names and sanitized warnings, in human or JSON command output.
// Constraints: preview returns before build/apply; dry-run remains offline.
// SPORT: D-0418.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/doctor"
	"github.com/spf13/cobra"
)

func init() {
	deployCmd.Flags().Bool("preview", false, "Compare local and host .env key names over SSH without deploying")
}

func runDeployEnvPreview(cmd *cobra.Command, workdir, target, filter string) error {
	checks := remoteEnvChecks(cmd.Context(), workdir, target, filter)
	if len(checks) == 0 {
		return fmt.Errorf("preview requires a configured remote environment and matching server")
	}
	if jsonRequested(cmd) {
		result := deployResult{Target: target, Strategy: "preview", Success: true}
		for _, check := range checks {
			result.Steps = append(result.Steps, deployStep{Name: check.Name + ": " + check.Message, Status: check.Status})
		}
		data := deployDataOf(result)
		data.DryRun = true
		return emitDeployDoc("deploy", result, data, nil)
	}
	for _, check := range checks {
		cmd.Printf("%s [%s]: %s\n", check.Name, check.Status, check.Message)
	}
	return nil
}

func remoteEnvChecks(ctx context.Context, workdir, target, filter string) []doctorCheckResult {
	inv, err := controlplane.Load(workdir)
	if err != nil {
		return []doctorCheckResult{{Name: "Host .env", Status: "warn", Message: "cannot load deploy inventory; comparison skipped"}}
	}
	name, ok := inventoryEnvName(inv, target)
	if !ok || inv.Environments[name].Kind == "local" {
		return nil
	}
	var checks []doctorCheckResult
	for _, srv := range inv.Environments[name].Servers {
		if srv.Host == "" || filter != "" && filter != srv.Name {
			continue
		}
		checks = append(checks, remoteEnvCheck(ctx, workdir, name, srv))
	}
	return checks
}

func remoteEnvCheck(ctx context.Context, workdir, target string, srv controlplane.Server) doctorCheckResult {
	result := doctorCheckResult{Name: "Host .env: " + srv.Name, Status: "warn"}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r, err := openDeployRemote(ctx, srv, os.Getenv(srv.SSHKeyRef), target)
	if err != nil {
		result.Message = "cannot open SSH transport; comparison skipped"
		return result
	}
	drift, err := deploy.CompareHostEnv(ctx, r, workdir, srv.RemotePath)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.Status, result.Message = "pass", "local and host .env match"
	if drift.Different() {
		result.Status = "warn"
		result.Message = fmt.Sprintf(".env keys: local-only [%s]; host-only [%s]; changed [%s]. Host .env stays authoritative; local .env is never shipped.", strings.Join(drift.LocalOnly, ", "), strings.Join(drift.HostOnly, ", "), strings.Join(drift.Changed, ", "))
	}
	return result
}

func checkDoctorRemoteEnv(ctx context.Context, cwd, selected string, display bool) []doctorCheckResult {
	checks := remoteEnvChecks(ctx, cwd, selected, "")
	if display {
		printDeepDoctorChecks(checks)
	}
	return checks
}

func doctorRemoteEnvDeepChecks(ctx context.Context, cwd, selected, only string, display bool, results []doctor.CheckResult) []doctor.CheckResult {
	if only != "" && only != "config" {
		return results
	}
	for _, r := range checkDoctorRemoteEnv(ctx, cwd, selected, display) {
		results = append(results, doctor.CheckResult{Section: "config", Name: r.Name, Status: r.Status, Message: r.Message})
	}
	return results
}
