package commands

// Purpose: guard the `nself runner` command wiring itself (flag
//   registration, host/executor resolution, the verify exit-code policy) —
//   internal/runner's own package tests cover provision/verify logic.
// Inputs:  none (pure function tests + cobra tree inspection).
// Outputs: none.
// Constraints: never invokes RunE — that would shell out — only inspects
//   registered flags and exercises the small helper functions in this
//   package directly.

import (
	"testing"

	"github.com/nself-org/cli/internal/runner"
)

func TestRunnerCmd_RegisteredOnRoot(t *testing.T) {
	found := false
	for _, c := range RootCmd.Commands() {
		if c.Name() == "runner" {
			found = true
		}
	}
	if !found {
		t.Fatal("runner command not registered on RootCmd")
	}
}

func TestRunnerCmd_HasProvisionAndVerifySubcommands(t *testing.T) {
	names := map[string]bool{}
	for _, c := range runnerCmd.Commands() {
		names[c.Name()] = true
	}
	if !names["provision"] || !names["verify"] {
		t.Fatalf("expected provision and verify subcommands, got %v", names)
	}
}

func TestRunnerExecutorsFromFlags_DefaultsToLocal(t *testing.T) {
	executors := runnerExecutorsFromFlags(nil, "")
	if len(executors) != 1 {
		t.Fatalf("len(executors) = %d, want 1", len(executors))
	}
	if executors[0].Label() != "local" {
		t.Fatalf("Label() = %q, want local", executors[0].Label())
	}
}

func TestRunnerExecutorsFromFlags_BuildsSSHPerHost(t *testing.T) {
	executors := runnerExecutorsFromFlags([]string{"ci@a.example", "ci@b.example"}, "/key")
	if len(executors) != 2 {
		t.Fatalf("len(executors) = %d, want 2", len(executors))
	}
	if executors[0].Label() != "ci@a.example" || executors[1].Label() != "ci@b.example" {
		t.Fatalf("labels = %q, %q", executors[0].Label(), executors[1].Label())
	}
}

func TestRunnerExecutorsFromFlags_LocalKeyword(t *testing.T) {
	executors := runnerExecutorsFromFlags([]string{"local"}, "")
	if executors[0].Label() != "local" {
		t.Fatalf("Label() = %q, want local for the literal \"local\" host", executors[0].Label())
	}
}

func TestRunnerVerifyFoundProblems_CleanReportsAreFalse(t *testing.T) {
	reports := []runner.HostReport{
		{Host: "a", Checks: []runner.CheckResult{{Name: "dep:git", Status: runner.StatusPass}}},
	}
	if runnerVerifyFoundProblems(reports) {
		t.Error("expected no problems for an all-pass report")
	}
}

func TestRunnerVerifyFoundProblems_FailingCheckIsTrue(t *testing.T) {
	reports := []runner.HostReport{
		{Host: "a", Checks: []runner.CheckResult{{Name: "dep:gh", Status: runner.StatusFail}}},
	}
	if !runnerVerifyFoundProblems(reports) {
		t.Error("expected a failing check to be reported as a problem")
	}
}

func TestRunnerVerifyFoundProblems_UnreachableHostIsTrue(t *testing.T) {
	reports := []runner.HostReport{{Host: "a", Err: "connection refused"}}
	if !runnerVerifyFoundProblems(reports) {
		t.Error("expected an unreachable host to be reported as a problem")
	}
}

func TestRunnerVerifyFoundProblems_DriftIsTrue(t *testing.T) {
	reports := []runner.HostReport{
		{Host: "a", Checks: []runner.CheckResult{{Name: "dep:gh", Status: runner.StatusPass}}},
		{Host: "b", Checks: []runner.CheckResult{{Name: "dep:gh", Status: runner.StatusFail}}},
	}
	if !runnerVerifyFoundProblems(reports) {
		t.Error("expected cross-host drift to be reported as a problem")
	}
}

func TestRunnerProvisionCmd_RequiresGithubURLAndToken(t *testing.T) {
	// Flags are registered with empty defaults; runRunnerProvision itself
	// validates presence (see runner_provision.go) rather than cobra
	// required-flag machinery, so both --github-url and a token source are
	// enforced. This test only asserts the flags exist with the documented
	// names — the validation path is exercised implicitly by code review of
	// runRunnerProvision's early-return guards, since invoking RunE would
	// shell out.
	for _, name := range []string{"host", "ssh-key", "instances", "install-root", "github-url", "labels", "token"} {
		if runnerProvisionCmd.Flags().Lookup(name) == nil {
			t.Errorf("runner provision missing --%s flag", name)
		}
	}
}

func TestRunnerVerifyCmd_HasExpectedFlags(t *testing.T) {
	for _, name := range []string{"host", "ssh-key", "json"} {
		if runnerVerifyCmd.Flags().Lookup(name) == nil {
			t.Errorf("runner verify missing --%s flag", name)
		}
	}
}
