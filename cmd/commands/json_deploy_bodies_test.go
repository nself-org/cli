package commands

// Purpose: body-level tests of the deploy fragment's v1 envelopes (P7-SURF-15):
// the run document of `deploy` in every path, status, check-access,
// environments, access list, the security documents and the failure rules.
// Inputs: the loopback fixtures of the deploy tests (scopeFixture, the fixture
// access transport); no remote host is contacted.
// Constraints: v1.5 mode is set per test; moved commands (deploy access, ...)
// run on the prepared tree so their registry key is the canonical path.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy/bluegreen"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// preparedTree puts RootCmd in its v1.5 shape for the test, so a moved command
// answers under its canonical path.
func preparedTree(t *testing.T) {
	t.Helper()
	compattest.Set(t, true)
	t.Cleanup(prepareTree(RootCmd, true, false))
	resetRegistryCache()
	t.Cleanup(resetRegistryCache)
}

func dataMap(t *testing.T, out, command string) map[string]json.RawMessage {
	t.Helper()
	doc := oneEnvelope(t, out, command)
	var data map[string]json.RawMessage
	if err := json.Unmarshal(doc["data"], &data); err != nil {
		t.Fatalf("%s data: %v\n%s", command, err, out)
	}
	return data
}

// TestDeployHubAndResultEnvelopes: a hub answers with its subcommands and a
// wrapped write command with the shared result, under the canonical path.
func TestDeployHubAndResultEnvelopes(t *testing.T) {
	preparedTree(t)
	out, err := jsonV15(t, accessCmd, map[string]string{"json": "true"}, func() error { return accessCmd.RunE(accessCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var hub dataHub
	if err := json.Unmarshal(dataRaw(t, jsonOnly(out), "deploy access"), &hub); err != nil || len(hub.Subcommands) != 3 {
		t.Fatalf("hub data %v: %v\n%s", hub, err, out)
	}
	if got := invokedKey(accessGrantCmd); got != "deploy access grant" {
		t.Fatalf("grant answers under %q", got)
	}
}

func dataRaw(t *testing.T, out, command string) json.RawMessage {
	t.Helper()
	return oneEnvelope(t, out, command)["data"]
}

// TestAccessListEnvelope: the fixture host answers as one row; NSELF_JSON_LEGACY=1
// keeps the bare array of v1.4.
func TestAccessListEnvelope(t *testing.T) {
	preparedTree(t)
	withFixtureTransport(t)
	ctx := context.Background()
	defer resetFlags(accessGrantCmd)
	accessGrantCmd.SetContext(ctx)
	_ = accessGrantCmd.Flags().Set("host", "root@fixture")
	_ = accessGrantCmd.Flags().Set("user", "alice")
	_ = accessGrantCmd.Flags().Set("key", testKeyLine)
	if _, err := captureStdout(t, func() error { return runAccessGrant(accessGrantCmd, nil) }); err != nil {
		t.Fatal(err)
	}
	accessListCmd.SetContext(ctx)
	defer resetFlags(accessListCmd)
	flags := map[string]string{"json": "true", "host": "root@fixture"}
	out, err := jsonV15(t, accessListCmd, flags, func() error { return runAccessList(accessListCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var data accessListData
	if err := json.Unmarshal(dataRaw(t, out, "deploy access list"), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Hosts) != 1 || data.Hosts[0].Status != "ok" || len(data.Hosts[0].Entries) != 1 || data.Hosts[0].Entries[0].User != "alice" {
		t.Fatalf("hosts: %+v", data.Hosts)
	}
	t.Setenv(output.LegacyEnvVar, "1")
	bare, err := jsonV15(t, accessListCmd, flags, func() error { return runAccessList(accessListCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var rows []accessListRow
	if err := json.Unmarshal([]byte(bare), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("legacy form is the bare array: %v\n%s", err, bare)
	}
}

// TestDeployFinalizeDocument: the run document in v1.5 is an envelope whose data
// marks a dry run; a failed run prints no data (the error envelope carries it),
// and NSELF_JSON_LEGACY=1 keeps the bare document, failure included.
func TestDeployFinalizeDocument(t *testing.T) {
	compattest.Set(t, true)
	steps := []deployStep{{Name: "Build images", Status: "pending"}}
	out, err := captureStdout(t, func() error { return finalize(true, "local", "rolling", time.Now(), steps, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var d deployData
	if err := json.Unmarshal(dataRaw(t, out, "deploy"), &d); err != nil || !d.DryRun || !d.Success || len(d.Steps) != 1 || d.Servers == nil {
		t.Fatalf("dry-run data %+v: %v\n%s", d, err, out)
	}
	boom := errors.New("build failed")
	out, err = captureStdout(t, func() error { return finalize(true, "local", "rolling", time.Now(), steps, boom) })
	if !errors.Is(err, boom) || strings.TrimSpace(out) != "" {
		t.Fatalf("failed run printed %q (err %v): the error envelope owns the failure", out, err)
	}
	t.Setenv(output.LegacyEnvVar, "1")
	out, err = captureStdout(t, func() error { return finalize(true, "local", "rolling", time.Now(), steps, boom) })
	var legacy deployResult
	if !errors.Is(err, boom) || json.Unmarshal([]byte(out), &legacy) != nil || legacy.Success || legacy.Error != "build failed" {
		t.Fatalf("legacy failure document %q (err %v)", out, err)
	}
	compattest.Set(t, false)
	out, err = captureStdout(t, func() error { return finalize(true, "local", "rolling", time.Now(), steps, nil) })
	if err != nil || json.Unmarshal([]byte(out), &legacy) != nil || legacy.Target != "local" {
		t.Fatalf("v1.4 document %q: %v", out, err)
	}
}

// TestDeployDryRunTopologyDocument: `deploy qa --dry-run --json` answers with the
// planned servers of the loopback inventory.
func TestDeployDryRunTopologyDocument(t *testing.T) {
	compattest.Set(t, true)
	scopeFixture(t, false)
	out, err := captureStdout(t, func() error {
		return runDeployArgs(t, map[string]string{"dry-run": "true", "json": "true"}, "qa")
	})
	if err != nil {
		t.Fatal(err)
	}
	var d deployData
	if err := json.Unmarshal(dataRaw(t, out, "deploy"), &d); err != nil {
		t.Fatal(err)
	}
	if !d.DryRun || d.Target != "qa" || len(d.Servers) != 1 || d.Servers[0].Status != "planned" || d.Servers[0].Capability == "" {
		t.Fatalf("topology data %+v", d)
	}
}

// TestDeployPipelineFailureHasNoData: an unreachable primary is an error, so no
// document reaches stdout in envelope mode.
func TestDeployPipelineFailureHasNoData(t *testing.T) {
	compattest.Set(t, true)
	scopeFixture(t, true)
	out, err := captureStdout(t, func() error {
		return runDeployArgs(t, map[string]string{"json": "true", "force": "true"}, "qa")
	})
	if err == nil || strings.TrimSpace(out) != "" {
		t.Fatalf("unreachable primary: err %v, stdout %q", err, out)
	}
}

// TestDeployPipelineAndBlueGreenData: the data builders map a run's outcome.
func TestDeployPipelineAndBlueGreenData(t *testing.T) {
	res := &controlplane.DeployResult{Servers: []controlplane.ServerResult{
		{Env: "qa", Server: "qa-app", Role: controlplane.RoleApp, Status: "ok"},
		{Env: "qa", Server: "qa-obs", Role: controlplane.RoleObservability, Status: "skipped"},
	}}
	d := deployPipelineData("qa", "rolling", time.Now(), res)
	if !d.Success || len(d.Servers) != 2 || d.Servers[1].Reason != "read-only capability" || d.Steps == nil {
		t.Fatalf("pipeline data %+v", d)
	}
	bg := deployBlueGreenData("local", false, bluegreen.DeployResult{Success: true, Steps: []bluegreen.DeployStep{{Name: "flip", Status: "done"}}, Duration: 1500 * time.Millisecond})
	if bg.Strategy != "blue-green" || bg.DurationMS != 1500 || len(bg.Steps) != 1 {
		t.Fatalf("blue/green data %+v", bg)
	}
}

// TestDeployStatusCheckAccessEnvironmentsDocuments: the read documents of a
// project whose inventory has only the local environment (nothing is probed).
func TestDeployStatusCheckAccessEnvironmentsDocuments(t *testing.T) {
	compattest.Set(t, true)
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)
	inv := &controlplane.Inventory{SchemaVersion: 1, Project: "t", Environments: map[string]controlplane.Environment{"local": {Name: "local", Kind: "local"}}}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatal(err)
	}
	c := func(cmd *cobra.Command) { cmd.SetContext(context.Background()) }

	c(deployStatusCmd)
	out, err := jsonV15(t, deployStatusCmd, map[string]string{"json": "true", "env": "local"}, func() error { return runDeployStatus(deployStatusCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	status := dataMap(t, out, "deploy status")
	if string(status["target"]) != `"local"` || string(status["servers"]) != "[]" {
		t.Fatalf("status data %v", status)
	}

	c(deployCheckAccessCmd)
	out, err = jsonV15(t, deployCheckAccessCmd, map[string]string{"json": "true"}, func() error { return runDeployCheckAccess(deployCheckAccessCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	// No inventory rows to probe: the document carries no servers and the env-var check says not ok (nothing set).
	if got := dataMap(t, out, "deploy check-access"); string(got["servers"]) != "[]" || string(got["all_ok"]) != "false" {
		t.Fatalf("check-access data %v", got)
	}

	c(deployEnvironmentsCmd)
	out, err = jsonV15(t, deployEnvironmentsCmd, map[string]string{"json": "true"}, func() error { return runDeployEnvironments(deployEnvironmentsCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if got := dataMap(t, out, "deploy environments"); !strings.Contains(string(got["environments"]), `"name": "local"`) {
		t.Fatalf("environments data %v", got)
	}
	// Without --json the document is the bare JSON it always was (it is the human output).
	_ = deployEnvironmentsCmd.Flags().Set("json", "false")
	bare, err := captureStdout(t, func() error { return runDeployEnvironments(deployEnvironmentsCmd, nil) })
	if err != nil || !strings.HasPrefix(bare, "{\n  \"environments\"") {
		t.Fatalf("plain run changed: %q %v", bare, err)
	}
	// NSELF_JSON_LEGACY=1 keeps the bare document with --json too.
	t.Setenv(output.LegacyEnvVar, "1")
	out, err = jsonV15(t, deployEnvironmentsCmd, map[string]string{"json": "true"}, func() error { return runDeployEnvironments(deployEnvironmentsCmd, nil) })
	if err != nil || !strings.HasPrefix(out, "{\n  \"environments\"") {
		t.Fatalf("legacy form is the bare document: %q %v", out, err)
	}
}

// TestSecurityDocuments: the audit and status checks run on this machine; the
// documents carry one finding per check, setup is a dry run without --apply.
func TestSecurityDocuments(t *testing.T) {
	preparedTree(t)
	out, err := jsonV15(t, securityAuditCmd, map[string]string{"json": "true"}, func() error { return securityAuditCmd.RunE(securityAuditCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var audit securityAuditData
	if err := json.Unmarshal(dataRaw(t, jsonOnly(out), "doctor security audit"), &audit); err != nil || len(audit.Findings) != len(runChecks()) || audit.Passed+audit.Failed != len(audit.Findings) {
		t.Fatalf("audit data %+v: %v", audit, err)
	}
	if got := newSecuritySetupData(true, true, nil, plannedHardeningSteps()); !got.DryRun || len(got.Steps) != len(plannedHardeningSteps()) || got.Steps[0].Command[0] == "" {
		t.Fatalf("setup data %+v", got)
	}
	if got := newSecuritySetupData(false, false, nil, nil); got.Steps == nil || got.Supported || got.Applied {
		t.Fatalf("unsupported setup data %+v", got)
	}
	// An applied run with a failed step is not "applied": the failure is named.
	if got := newSecuritySetupData(false, true, []string{"Enable ufw"}, plannedHardeningSteps()); got.Applied || len(got.Failed) != 1 {
		t.Fatalf("partial setup data %+v", got)
	}
	if got := newSecuritySetupData(false, true, nil, plannedHardeningSteps()); !got.Applied || got.Failed == nil {
		t.Fatalf("full setup data %+v", got)
	}
}

// TestUpdateUpgradeData names the requested operation.
func TestUpdateUpgradeData(t *testing.T) {
	for _, tc := range []struct {
		check, rollback, url bool
		want                 string
	}{{true, false, false, "check"}, {false, true, false, "rollback"}, {false, false, true, "binary-url"}, {false, false, false, "upgrade"}} {
		if got := newUpdateUpgradeData(tc.check, tc.rollback, tc.url, "").Action; got != tc.want {
			t.Errorf("%+v: action %q, want %q", tc, got, tc.want)
		}
	}
}

// TestAfterDocumentIsSilentOnlyInEnvelopeMode: a failure after the run document
// was printed keeps its exit status but prints no error envelope on stdout.
func TestAfterDocumentIsSilentOnlyInEnvelopeMode(t *testing.T) {
	boom := errors.New("follow failed")
	compattest.Set(t, true)
	var silencer errs.Silencer
	got := afterDocument(true, boom)
	if !errors.Is(got, boom) || !errors.As(got, &silencer) || !silencer.Silent() {
		t.Fatalf("envelope mode: %v is not a silent wrapper of the failure", got)
	}
	if got := afterDocument(false, boom); got != boom {
		t.Fatalf("no --json: %v", got)
	}
	compattest.Set(t, false)
	if got := afterDocument(true, boom); got != boom {
		t.Fatalf("v1.4: %v", got)
	}
	if afterDocument(true, nil) != nil {
		t.Fatal("nil stays nil")
	}
}
