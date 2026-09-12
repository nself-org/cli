package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestVerifyHosts_TwoHostsIdentical_NoDrift models the healthy fleet case:
// both hosts pass every check.
func TestVerifyHosts_TwoHostsIdentical_NoDrift(t *testing.T) {
	m := testManifest()
	host1 := &fakeExecutor{label: "runner-1"}
	host1.when("echo reachable", "reachable", nil)
	host1.when("command -v 'git'", "/usr/bin/git", nil)
	host1.when("dpkg -s 'libssl-dev'", "installed", nil)
	host1.when("if [ -L", "DIR", nil)

	host2 := &fakeExecutor{label: "runner-2"}
	host2.when("echo reachable", "reachable", nil)
	host2.when("command -v 'git'", "/usr/bin/git", nil)
	host2.when("dpkg -s 'libssl-dev'", "installed", nil)
	host2.when("if [ -L", "DIR", nil)

	reports := VerifyHosts(context.Background(), []Executor{host1, host2}, m)
	if len(reports) != 2 {
		t.Fatalf("len(reports) = %d, want 2", len(reports))
	}
	drift := DetectDrift(reports)
	if len(drift) != 0 {
		t.Fatalf("expected no drift, got %+v", drift)
	}
}

// TestVerifyHosts_DetectsLabelDrift is the exact scenario from the G-012
// evidence: two hosts advertise the same GitHub Actions labels but one is
// missing `gh` — verify must surface that as drift, not silently pass.
func TestVerifyHosts_DetectsLabelDrift(t *testing.T) {
	m := &Manifest{Dependencies: []Dependency{{Name: "gh", AptPackage: "gh", Binary: "gh"}}}

	hostWithGH := &fakeExecutor{label: "runner-a"}
	hostWithGH.when("echo reachable", "reachable", nil)
	hostWithGH.when("command -v 'gh'", "/usr/bin/gh", nil)

	hostWithoutGH := &fakeExecutor{label: "runner-b"}
	hostWithoutGH.when("echo reachable", "reachable", nil)
	hostWithoutGH.fallback = fakeResponse{out: "", err: errors.New("exit status 1")}

	reports := VerifyHosts(context.Background(), []Executor{hostWithGH, hostWithoutGH}, m)
	drift := DetectDrift(reports)
	if len(drift) != 1 {
		t.Fatalf("expected exactly one drift finding, got %+v", drift)
	}
	if drift[0].CheckName != "dep:gh" {
		t.Fatalf("drift on wrong check: %+v", drift[0])
	}
	if drift[0].ByHost["runner-a"] != StatusPass || drift[0].ByHost["runner-b"] != StatusFail {
		t.Fatalf("drift byHost = %+v", drift[0].ByHost)
	}

	matrix := RenderMatrix(reports)
	if !strings.Contains(matrix, "DRIFT DETECTED") {
		t.Fatalf("matrix should report drift:\n%s", matrix)
	}
	if !strings.Contains(matrix, "dep:gh") {
		t.Fatalf("matrix should name the drifted check:\n%s", matrix)
	}
}

func TestVerifyHosts_UnreachableHostExcludedFromDrift(t *testing.T) {
	m := &Manifest{Dependencies: []Dependency{{Name: "gh", AptPackage: "gh", Binary: "gh"}}}

	reachable := &fakeExecutor{label: "runner-a"}
	reachable.when("echo reachable", "reachable", nil)
	reachable.when("command -v 'gh'", "/usr/bin/gh", nil)

	unreachable := &fakeExecutor{label: "runner-down", fallback: fakeResponse{err: errors.New("dial tcp: connection refused")}}

	reports := VerifyHosts(context.Background(), []Executor{reachable, unreachable}, m)
	var downReport *HostReport
	for i := range reports {
		if reports[i].Host == "runner-down" {
			downReport = &reports[i]
		}
	}
	if downReport == nil || downReport.Err == "" {
		t.Fatalf("expected runner-down to carry a non-empty Err, got %+v", downReport)
	}
	if len(downReport.Checks) != 0 {
		t.Fatalf("unreachable host should have zero Checks, got %d", len(downReport.Checks))
	}
	if drift := DetectDrift(reports); len(drift) != 0 {
		t.Fatalf("unreachable host must not be reported as drift, got %+v", drift)
	}
}

func TestRenderMatrix_NoDrift(t *testing.T) {
	reports := []HostReport{
		{Host: "a", Checks: []CheckResult{{Name: "dep:git", Status: StatusPass}}},
		{Host: "b", Checks: []CheckResult{{Name: "dep:git", Status: StatusPass}}},
	}
	out := RenderMatrix(reports)
	if !strings.Contains(out, "No drift detected") {
		t.Fatalf("expected no-drift message:\n%s", out)
	}
}

func TestAllEqual(t *testing.T) {
	if !allEqual(map[string]CheckStatus{"a": StatusPass, "b": StatusPass}) {
		t.Error("expected equal statuses to report true")
	}
	if allEqual(map[string]CheckStatus{"a": StatusPass, "b": StatusFail}) {
		t.Error("expected differing statuses to report false")
	}
}
