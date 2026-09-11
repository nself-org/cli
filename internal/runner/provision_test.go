package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestProvision_RunsAllStepsInOrder(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "ok", err: nil}}
	m := testManifest()

	result, err := Provision(context.Background(), ex, m, ProvisionOptions{
		Instances: 1,
		GithubURL: "https://github.com/nself-org/cli",
		RegToken:  "AREGTOKEN",
	})
	if err != nil {
		t.Fatalf("Provision returned error: %v", err)
	}
	wantSteps := []string{
		"ensure-gh-apt-repo", "install-packages", "create-runner-user",
		"grant-passwordless-sudo", "ensure-work-dir", "install-runner-1",
	}
	if len(result.Steps) != len(wantSteps) {
		t.Fatalf("got %d steps, want %d: %+v", len(result.Steps), len(wantSteps), result.Steps)
	}
	for i, name := range wantSteps {
		if result.Steps[i].Name != name {
			t.Errorf("step %d = %q, want %q", i, result.Steps[i].Name, name)
		}
	}
}

func TestProvision_MultipleInstances(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "ok", err: nil}}
	result, err := Provision(context.Background(), ex, testManifest(), ProvisionOptions{Instances: 3})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	instanceSteps := 0
	for _, s := range result.Steps {
		if strings.HasPrefix(s.Name, "install-runner-") {
			instanceSteps++
		}
	}
	if instanceSteps != 3 {
		t.Fatalf("instance steps = %d, want 3", instanceSteps)
	}
}

func TestProvision_StopsOnFirstFailure(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "ok", err: nil}}
	ex.when("apt-get install", "", errors.New("apt-get exit 100"))

	result, err := Provision(context.Background(), ex, testManifest(), ProvisionOptions{Instances: 2})
	if err == nil {
		t.Fatal("expected an error when install-packages fails")
	}
	if !strings.Contains(err.Error(), "install-packages") {
		t.Fatalf("error should name the failing step, got: %v", err)
	}
	for _, s := range result.Steps {
		if strings.HasPrefix(s.Name, "install-runner-") {
			t.Fatalf("no runner instance should be installed after an earlier step failed, got %q", s.Name)
		}
	}
}

func TestEnsureRealWorkDir_RefusesSymlink(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "", err: errors.New("exit status 1: REFUSING: symlink")}}
	_, err := ensureRealWorkDir(context.Background(), ex, "/opt/actions-runner/_work")
	if err == nil {
		t.Fatal("expected ensureRealWorkDir to surface the refusal error")
	}
}

func TestEnsureRealWorkDir_Unconfigured(t *testing.T) {
	out, err := ensureRealWorkDir(context.Background(), &fakeExecutor{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "skipped") {
		t.Fatalf("out = %q", out)
	}
}

func TestEnsurePasswordlessSudo_CommandShape(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "passwordless sudo granted", err: nil}}
	_, err := ensurePasswordlessSudo(context.Background(), ex, "gha-runner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ex.commands) != 1 {
		t.Fatalf("expected exactly one command, got %d", len(ex.commands))
	}
	cmd := ex.commands[0]
	for _, want := range []string{"/etc/sudoers.d/gha-runner-nself-runner", "NOPASSWD:ALL", "visudo -cf"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("sudoers command missing %q:\n%s", want, cmd)
		}
	}
}

func TestInstallRunnerInstance_LabelsIncludeSelfHostedDefaults(t *testing.T) {
	ex := &fakeExecutor{label: "runner-a", fallback: fakeResponse{out: "ok", err: nil}}
	_, err := installRunnerInstance(context.Background(), ex, testManifest(), ProvisionOptions{
		InstallRoot: "/opt/actions-runner",
		GithubURL:   "https://github.com/nself-org/cli",
		RegToken:    "TOKEN",
		Labels:      []string{"nself-ci"},
	}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmd := ex.commands[0]
	for _, want := range []string{"self-hosted,Linux,X64,nself-ci", "runner-a-1", "runner-1", "--unattended"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("instance command missing %q:\n%s", want, cmd)
		}
	}
}
