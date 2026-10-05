package commands

// deploy_run_pipeline.go — T05 control-plane pipeline deploy path for
// `nself deploy`.
//
// Purpose: active when .nself/control-plane.yaml exists OR --server is
//          specified; runs the topology-aware multi-server pipeline instead
//          of the legacy single-host path. Split out of deploy_run.go
//          (T-P6-E2-W1-S1-T3) for 300-line compliance.
// Inputs:  the relevant runDeploy flag values + cmd/workdir/target/strategy.
// Outputs: handled=false when neither control-plane.yaml nor --server is
//          present — caller runs the legacy single-host path unchanged
//          (back-compat byte-identical guarantee). handled=true means this
//          function fully handled the deploy and its err is runDeploy's result.
// Constraints: pure move, same checks/output/errors, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// prodClassFn decides whether an environment needs the production confirmation.
// A variable so a test can mark a custom name prod-class before P7-DEPL-13
// adds tier lookup to controlplane.IsProdClass.
var prodClassFn = controlplane.IsProdClass

// newDeployProber builds the SSH prober for a deploy. A variable so tests can
// record which hosts are probed instead of contacting them.
var newDeployProber = func(workdir string) controlplane.Prober {
	return controlplane.NewSSHProber(workdir, false)
}

// remoteHasuraStrict is the default for failing a deploy on a Hasura metadata
// error: strict on staging and on every prod-class environment, warn-only
// elsewhere. It keys off the environment, not a fixed pair of names.
func remoteHasuraStrict(workdir, target string) bool {
	inv, _ := controlplane.Load(workdir)
	return target == "staging" || prodClassFn(inv, target)
}

// inventoryEnvName finds env in inv by case-insensitive name. It reports false
// for a nil inventory, no match, or when two inventory keys differ only by
// case (even if one matches exactly): the caller refuses rather than guesses.
func inventoryEnvName(inv *controlplane.Inventory, env string) (string, bool) {
	if inv == nil {
		return "", false
	}
	found := ""
	for k := range inv.Environments {
		if strings.EqualFold(k, env) {
			if found != "" {
				return "", false
			}
			found = k
		}
	}
	return found, found != ""
}

// knownDeployEnvs lists the environments a deploy target may name, sorted:
// local, the legacy staging and prod names, and every inventory environment.
func knownDeployEnvs(inv *controlplane.Inventory) []string {
	set := map[string]bool{"local": true, "staging": true, "prod": true}
	if inv != nil {
		for k := range inv.Environments {
			set[k] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// deployHostEnvVar returns the NSELF_DEPLOY_HOST_<ENV> variable name for env.
func deployHostEnvVar(env string) string {
	return "NSELF_DEPLOY_HOST_" + strings.ToUpper(strings.ReplaceAll(env, "-", "_"))
}

// legacyDeployHost reads the single-host address for env from the environment.
func legacyDeployHost(env string) string {
	if h := os.Getenv(deployHostEnvVar(env)); h != "" {
		return h
	}
	return os.Getenv(strings.ToUpper(strings.ReplaceAll(env, "-", "_")) + "_DEPLOY_HOST")
}

// scopeInventoryToEnv returns inv reduced to the target environment only, or
// E483 when the environment is not in it. Called before any probe or deploy.
func scopeInventoryToEnv(inv *controlplane.Inventory, target string) (*controlplane.Inventory, error) {
	scoped, err := controlplane.ScopeToEnv(inv, target)
	if err != nil {
		return nil, errs.New("E483", fmt.Sprintf("invalid target %q: not in the deploy inventory (known: %s)", target, strings.Join(knownDeployEnvs(inv), ", ")))
	}
	return scoped, nil
}

// failedServersError returns a non-nil error naming every server whose deploy
// failed, so a partly or wholly failed pipeline never exits 0.
func failedServersError(r *controlplane.DeployResult) error {
	var failed []string
	for _, sr := range r.Servers {
		if sr.Status == "failed" {
			failed = append(failed, fmt.Sprintf("%s/%s: %v", sr.Env, sr.Server, sr.Err))
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("deploy: %d server(s) failed: %s", len(failed), strings.Join(failed, "; "))
}

// runDeployControlPlanePipeline handles the T05 pipeline deploy path. See
// file header for the handled/err contract.
func runDeployControlPlanePipeline(cmd *cobra.Command, workdir, target, strategy, serverFilter string, dryRun, jsonOut bool) (handled bool, err error) {
	// T05: Control-plane pipeline path.
	// Active when .nself/control-plane.yaml exists OR when --server is specified.
	// When neither condition is true the legacy single-host path runs unchanged
	// (back-compat byte-identical guarantee per sprint spec).
	cpYamlPath := filepath.Join(workdir, ".nself", "control-plane.yaml")
	_, cpYamlErr := os.Stat(cpYamlPath)
	usePipeline := cpYamlErr == nil || serverFilter != ""
	if !usePipeline {
		return false, nil
	}
	// "local" is the build-and-start path on this machine; the pipeline has no
	// local primitive. It must not fall into the pipeline, which would deploy
	// remote environments for a local request.
	if target == "local" && serverFilter == "" {
		return false, nil
	}

	inv, loadErr := controlplane.Load(workdir)
	if loadErr != nil {
		return true, fmt.Errorf("deploy: load inventory: %w", loadErr)
	}

	// Scope to the named environment first: a deploy of one env must never see
	// another env's servers, in the --server filter, the dry-run probe or Run.
	inv, scopeErr := scopeInventoryToEnv(inv, target)
	if scopeErr != nil {
		return true, scopeErr
	}

	// Apply server filter: remove all servers that do not match the requested name.
	if serverFilter != "" {
		inv = filterInventoryByServer(inv, serverFilter)
		if totalServers(inv) == 0 {
			return true, fmt.Errorf("deploy: --server %q not found in environment %q", serverFilter, target)
		}
	}

	// --dry-run: print topology plan and exit without executing.
	if dryRun {
		prober := newDeployProber(workdir)
		statuses := controlplane.Resolve(inv, prober)
		if !jsonOut {
			fmt.Printf("  [dry-run] Topology plan for target %q:\n", target)
			for _, ts := range statuses {
				if ts.Capability == controlplane.CapHidden {
					continue
				}
				fmt.Printf("    %s/%s role=%-15s capability=%s", ts.Env, ts.Server, "", string(ts.Capability))
				if ts.Reason != "" {
					fmt.Printf(" reason=%q", ts.Reason)
				}
				fmt.Println()
			}
		} else {
			type dryRow struct {
				Env        string `json:"env"`
				Server     string `json:"server"`
				Capability string `json:"capability"`
				Reason     string `json:"reason,omitempty"`
			}
			var rows []dryRow
			for _, ts := range statuses {
				if ts.Capability == controlplane.CapHidden {
					continue
				}
				rows = append(rows, dryRow{Env: ts.Env, Server: ts.Server, Capability: string(ts.Capability), Reason: ts.Reason})
			}
			b, _ := json.MarshalIndent(map[string]interface{}{"dry_run": true, "topology": rows}, "", "  ")
			fmt.Println(string(b))
		}
		return true, nil
	}

	// A remote pipeline deploy ships a fresh remote build and the validated env
	// snapshot, never whatever compose a previous (possibly local) build left.
	// Either failing stops here, before any probe or remote change.
	envFile, envName := "", ""
	if target != "local" {
		if _, buildErr := deployBuildStepFn(cmd.Context(), workdir, true, nil); buildErr != nil {
			return true, fmt.Errorf("deploy: remote build failed, nothing was sent: %w", buildErr)
		}
		envName = deployCascadeEnv(workdir, target)
		snap, cleanup, snapErr := writeResolvedDeployEnv(workdir, envName)
		if snapErr != nil {
			return true, fmt.Errorf("deploy: env snapshot failed, nothing was sent: %w", snapErr)
		}
		defer cleanup()
		envFile = snap
	}

	// Execute via topology-aware pipeline.
	prober := newDeployProber(workdir)
	composePath := filepath.Join(workdir, "docker-compose.yml")

	if !jsonOut {
		ui.CommandHeader(fmt.Sprintf("nself deploy %s (pipeline)", target), fmt.Sprintf("strategy=%s server=%s", strategy, serverFilter))
	}

	result, pipeErr := controlplane.RunWithEnv(cmd.Context(), inv, target, prober, composePath, envFile, envName)
	if pipeErr != nil {
		return true, fmt.Errorf("deploy pipeline: %w", pipeErr)
	}

	// Primary-skip gate: non-zero exit when primary was skipped.
	if result.PrimarySkipped {
		if !jsonOut {
			ui.Warn("Primary server was skipped (read-only capability) — deploy incomplete")
		} else {
			b, _ := json.MarshalIndent(result.Servers, "", "  ")
			fmt.Println(string(b))
		}
		return true, fmt.Errorf("deploy: primary server skipped (read-only capability); re-run once SSH access is restored")
	}

	failedErr := failedServersError(result)
	if jsonOut {
		b, _ := json.MarshalIndent(result.Servers, "", "  ")
		fmt.Println(string(b))
	} else {
		for _, sr := range result.Servers {
			switch sr.Status {
			case "ok":
				ui.Success(fmt.Sprintf("  [ok] %s/%s", sr.Env, sr.Server))
			case "skipped":
				ui.Warn(fmt.Sprintf("  [skipped] %s/%s (read-only)", sr.Env, sr.Server))
			case "failed":
				ui.Error(fmt.Sprintf("  [failed] %s/%s: %v", sr.Env, sr.Server, sr.Err))
			}
		}
		if failedErr == nil {
			ui.Success(fmt.Sprintf("Deploy %s (pipeline) complete", target))
		}
	}
	return true, failedErr
}
