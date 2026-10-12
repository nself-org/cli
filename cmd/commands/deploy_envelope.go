package commands

// Purpose: the v1 envelope wiring of the deploy canon fragment (deploy, deploy
// status|health|check-access|environments|access|ops|web|rollback|security,
// doctor security, update images|upgrade|verify-sbom; P7-SURF-15). It wraps
// the RunE of the commands that answer with the shared result or hub (dataEnv,
// db_envelope.go) and holds the emit helpers of the commands that already
// printed JSON before the contract.
// Inputs: the cobra commands of the fragment, wrapped from init(); facts the
// bodies hand over with setData or the emit helpers below.
// Outputs: one envelope on stdout in v1.5 mode with --json; v1.4 output and
// every human output are unchanged. NSELF_JSON_LEGACY=1 keeps the bare
// pre-contract document of the commands that had one.
// Constraints: a failed run prints the error envelope from the top-level
// handler, never a data document; `--stream` runs print no document (the
// stream's last line is the result); child-process output is never captured;
// `deploy promote` (P7-DEPL-15) and `deploy targets` (P7-DEPL-13) are not here.
// SPORT: P7-SURF-15, contract:cli.json-envelope

import (
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy/bluegreen"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/version"
	"github.com/spf13/cobra"
)

// deployDataEnvCmds are the commands whose RunE is wrapped with dataEnv: the
// ones that hand their data over with setData or take the shared result/hub.
// The self-emitting rows (deploy, status, health-less reads, check-access,
// environments, access list, promote-env) call the emit helpers themselves.
func deployDataEnvCmds() []*cobra.Command {
	return []*cobra.Command{
		accessCmd, accessGrantCmd, accessRevokeCmd,
		opsDeployCmd, deployRollbackCmd, deployWebCmd, deployHealthCmd,
		securitySetupCmd, securityAuditCmd, securityStatusCmd,
		updateImagesCmd, upgradeCmd, verifySBOMCmd, promoteRollbackCmd,
	}
}

func init() {
	for _, c := range deployDataEnvCmds() {
		c.RunE = dataEnv(c.RunE)
	}
}

// deployDocEnvelope reports whether a self-emitting command answers with the
// v1 envelope: v1.5 mode and the bare pre-contract form not requested.
func deployDocEnvelope() bool {
	// compat.V15(P7-SURF-15): deploy, status, check-access, access list --json print a bare document (NSELF_JSON_LEGACY=1 keeps it) -> v1 envelope
	return compat.V15() && !legacyJSONRequested()
}

// emitDeployDoc prints the JSON document of a deploy-fragment command that
// printed one before the contract. legacy is the pre-contract value (printed
// bare in v1.4 and with NSELF_JSON_LEGACY=1), data the envelope data. A run
// error returns unprinted in envelope mode: the error envelope carries it.
func emitDeployDoc(command string, legacy, data any, runErr error) error {
	if deployDocEnvelope() {
		if runErr != nil {
			return runErr
		}
		return output.EmitData(output.Default(), command, data)
	}
	if err := output.EmitLegacyCompatible(output.Default(), command, legacy); err != nil && runErr == nil {
		return err
	}
	return runErr
}

// deployDataOf converts the legacy run document to the envelope data. A step
// is only "pending" in a dry run (stepStatus), which marks the whole run.
func deployDataOf(res deployResult) deployData {
	d := deployData{Target: res.Target, Strategy: res.Strategy, Steps: res.Steps, Servers: []deployServerRow{},
		DurationMS: res.Duration, Success: res.Success}
	if d.Steps == nil {
		d.Steps = []deployStep{}
	}
	for _, s := range res.Steps {
		if s.Status == "pending" {
			d.DryRun = true
		}
	}
	return d
}

// deployTopologyData is the dry-run plan of a pipeline deploy: one planned
// row per visible server.
func deployTopologyData(target, strategy string, statuses []controlplane.TargetStatus) deployData {
	d := deployData{Target: target, Strategy: strategy, DryRun: true, Steps: []deployStep{}, Servers: []deployServerRow{}, Success: true}
	for _, ts := range statuses {
		if ts.Capability == controlplane.CapHidden {
			continue
		}
		d.Servers = append(d.Servers, deployServerRow{Env: ts.Env, Server: ts.Server, Status: "planned", Capability: string(ts.Capability), Reason: ts.Reason})
	}
	return d
}

// deployPipelineData is the outcome of a pipeline deploy that did not fail.
func deployPipelineData(target, strategy string, start time.Time, res *controlplane.DeployResult) deployData {
	d := deployData{Target: target, Strategy: strategy, Steps: []deployStep{}, Servers: []deployServerRow{},
		DurationMS: time.Since(start).Milliseconds(), Success: true}
	for _, sr := range res.Servers {
		row := deployServerRow{Env: sr.Env, Server: sr.Server, Role: string(sr.Role), Status: sr.Status}
		if sr.Status == "skipped" {
			row.Reason = "read-only capability"
		}
		d.Servers = append(d.Servers, row)
	}
	return d
}

// deployBlueGreenData is the outcome of a blue/green deploy that succeeded.
func deployBlueGreenData(target string, dryRun bool, res bluegreen.DeployResult) deployData {
	d := deployData{Target: target, Strategy: "blue-green", DryRun: dryRun, Steps: []deployStep{}, Servers: []deployServerRow{},
		DurationMS: res.Duration.Milliseconds(), Success: res.Success}
	for _, s := range res.Steps {
		d.Steps = append(d.Steps, deployStep{Name: s.Name, Status: s.Status})
	}
	return d
}

// emitTopologyDoc prints the dry-run topology plan of a pipeline deploy. The
// pre-contract document is {"dry_run":true,"topology":[...]}.
func emitTopologyDoc(target, strategy string, statuses []controlplane.TargetStatus) error {
	var rows []deployAccessRow
	for _, ts := range statuses {
		if ts.Capability != controlplane.CapHidden {
			rows = append(rows, deployAccessRow{Env: ts.Env, Server: ts.Server, Capability: string(ts.Capability), Reason: ts.Reason})
		}
	}
	legacy := map[string]any{"dry_run": true, "topology": rows}
	return emitDeployDoc("deploy", legacy, deployTopologyData(target, strategy, statuses), nil)
}

// newDeployStatusData builds the `deploy status` document; rows is nil when
// the project has no control-plane inventory.
func newDeployStatusData(target, state string, rows []deployStatusServer) deployStatusData {
	if rows == nil {
		rows = []deployStatusServer{}
	}
	return deployStatusData{Target: target, State: state, Servers: rows}
}

// newDeployCheckAccessData builds the `deploy check-access` document; rows is
// nil when the project has no control-plane inventory (the env-var check).
func newDeployCheckAccessData(rows []deployAccessRow, allOK bool) deployCheckAccessData {
	if rows == nil {
		rows = []deployAccessRow{}
	}
	return deployCheckAccessData{Servers: rows, AllOK: allOK}
}

// newSecurityFindings converts the audit checks to envelope rows.
func newSecurityFindings(fs []finding) []securityFinding {
	out := make([]securityFinding, 0, len(fs))
	for _, f := range fs {
		out = append(out, securityFinding(f))
	}
	return out
}

// newSecuritySetupData builds the `deploy security setup` document from the
// planned hardening steps (nil off Linux). failed names the steps that ran and
// failed; the run applied everything only when none did and it was not a dry run.
func newSecuritySetupData(dryRun, supported bool, failed []string, steps []hardeningStep) securitySetupData {
	d := securitySetupData{DryRun: dryRun, Supported: supported, Failed: append([]string{}, failed...), Steps: []securitySetupStep{}}
	d.Applied = supported && !dryRun && len(failed) == 0
	for _, s := range steps {
		d.Steps = append(d.Steps, securitySetupStep{Name: s.Name, Command: append([]string{}, s.Cmd...)})
	}
	return d
}

// newUpdateUpgradeData names the operation `update upgrade` was asked for.
func newUpdateUpgradeData(check, rollback, binaryURL bool, target string) updateUpgradeData {
	action := "upgrade"
	switch {
	case rollback:
		action = "rollback"
	case binaryURL:
		action = "binary-url"
	case check:
		action = "check"
	}
	return updateUpgradeData{OK: true, Action: action, CurrentVersion: version.GetVersion(), TargetVersion: target}
}
