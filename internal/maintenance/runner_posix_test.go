//go:build darwin || linux

package maintenance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withoutStalenessGuard disables the workspace staleness window for tests whose
// fixtures are necessarily brand new. Those tests assert what reclaimRunnerWork
// removes and protects, not when it defers; the staleness behaviour has its own
// test (TestReclaimRunnerWork_SkipsRecentlyModifiedWorkspace).
func withoutStalenessGuard(t *testing.T) {
	t.Helper()
	prev := workspaceStaleAfter
	workspaceStaleAfter = 0
	t.Cleanup(func() { workspaceStaleAfter = prev })
}

// mustMkdirWithFile creates dir and a small file inside it so dirSize/removal is
// exercising real bytes, not an empty directory.
func mustMkdirWithFile(t *testing.T, dir string, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "payload"), []byte(content), 0o644); err != nil {
		t.Fatalf("write payload in %s: %v", dir, err)
	}
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

// withNoBusyRunners stubs listRunnerWorkerProcesses to report no runner processes at
// all, and restores the original on cleanup. Tests never shell out to a real `ps` or
// touch a real runner.
func withNoBusyRunners(t *testing.T) {
	t.Helper()
	orig := listRunnerWorkerProcesses
	listRunnerWorkerProcesses = func() ([]string, error) { return nil, nil }
	t.Cleanup(func() { listRunnerWorkerProcesses = orig })
}

// withBusyRunner stubs listRunnerWorkerProcesses to report a live Runner.Worker whose
// path is under busyRoot, and restores the original on cleanup.
func withBusyRunner(t *testing.T, busyRoot string) {
	t.Helper()
	orig := listRunnerWorkerProcesses
	listRunnerWorkerProcesses = func() ([]string, error) {
		return []string{filepath.Join(busyRoot, "bin", "Runner.Worker") + " spawnclient"}, nil
	}
	t.Cleanup(func() { listRunnerWorkerProcesses = orig })
}

// withStubbedSideEffects replaces the docker/log-rotation/journald hooks with no-ops
// for the duration of the test. DiskCleanupWithOptions always runs the "always safe"
// tier, and these tests must never shell out to a real docker daemon, /var/log, or
// journald — only the filesystem-fixture-driven tiers (runner work, caches) are under
// test here.
func withStubbedSideEffects(t *testing.T) {
	t.Helper()
	origDocker := dockerReclaimFunc
	origLog := logRotationFunc
	origJournal := journalVacuumFunc
	dockerReclaimFunc = func(bool) (string, []error) { return "stubbed: docker not touched in tests", nil }
	logRotationFunc = func(bool) (string, error) { return "stubbed: /var/log not touched in tests", nil }
	journalVacuumFunc = func(bool) (string, error) { return "stubbed: journald not touched in tests", nil }
	t.Cleanup(func() {
		dockerReclaimFunc = origDocker
		logRotationFunc = origLog
		journalVacuumFunc = origJournal
	})
}

// ── _actions/_tool/_temp/_PipelineMapping preserved ────────────────────────────────

func TestReclaimRunnerWork_PreservesProtectedSubdirs(t *testing.T) {
	withoutStalenessGuard(t)
	root := t.TempDir()
	work := filepath.Join(root, "_work")

	for _, protected := range []string{"_actions", "_tool", "_temp", "_PipelineMapping"} {
		mustMkdirWithFile(t, filepath.Join(work, protected), "runner internals")
	}
	// reclaimRunnerWork only inspects DIRECT children of _work (matching the real
	// runner layout, "_work/<repo>/<repo>") — so the removable unit here is the
	// top-level "nself-org-cli" directory, not a nested path within it.
	jobDir := filepath.Join(work, "nself-org-cli")
	mustMkdirWithFile(t, jobDir, "checked out job files")

	bytes, reclaimed, skipped := reclaimRunnerWork(root, false)

	for _, protected := range []string{"_actions", "_tool", "_temp", "_PipelineMapping"} {
		p := filepath.Join(work, protected)
		if !exists(t, p) {
			t.Errorf("protected dir %s was removed; must never be touched", p)
		}
	}
	if exists(t, jobDir) {
		t.Errorf("job workspace dir %s was not removed", jobDir)
	}
	if bytes == 0 {
		t.Error("expected non-zero bytes reclaimed from the job workspace")
	}
	if len(reclaimed) != 1 || reclaimed[0].Path != jobDir {
		t.Errorf("reclaimed = %+v; want exactly the job dir", reclaimed)
	}
	if len(skipped) != 4 {
		t.Errorf("skipped = %d entries; want 4 (one per protected subdir)", len(skipped))
	}
	for _, s := range skipped {
		if !strings.Contains(s.Reason, "never removed") {
			t.Errorf("skip reason for %s = %q; want it to explain the permanent exclusion", s.Path, s.Reason)
		}
	}
}

// ── go-build / grype / trivy preserved, unconditionally ────────────────────────────

func TestReclaimCaches_PreservesGoBuildGrypeTrivy(t *testing.T) {
	withoutStalenessGuard(t)
	home := t.TempDir()
	mustMkdirWithFile(t, filepath.Join(home, ".cache", "go-build"), "compiled object")
	mustMkdirWithFile(t, filepath.Join(home, ".cache", "grype"), "vuln db")
	mustMkdirWithFile(t, filepath.Join(home, ".cache", "trivy"), "vuln db")
	mustMkdirWithFile(t, filepath.Join(home, ".cache", "turbo"), "turborepo cache")
	mustMkdirWithFile(t, filepath.Join(home, "go", "pkg", "mod"), "downloaded module")
	mustMkdirWithFile(t, filepath.Join(home, "pnpm-store"), "content-addressable store")

	bytes, reclaimed, skipped := reclaimCaches(home, []string{}, false)

	for _, protected := range []string{"go-build", "grype", "trivy"} {
		p := filepath.Join(home, ".cache", protected)
		if !exists(t, p) {
			t.Errorf("protected cache %s was removed; compiler/scanner inputs must never be touched", p)
		}
		if !exists(t, filepath.Join(p, "payload")) {
			t.Errorf("protected cache %s had its contents removed", p)
		}
	}

	for _, removable := range []string{
		filepath.Join(".cache", "turbo"),
		filepath.Join("go", "pkg", "mod"),
		"pnpm-store",
	} {
		p := filepath.Join(home, removable)
		if exists(t, p) {
			t.Errorf("regenerable cache %s was not removed", p)
		}
	}

	if bytes == 0 {
		t.Error("expected non-zero bytes reclaimed from regenerable caches")
	}
	if len(reclaimed) != 3 {
		t.Errorf("reclaimed = %d entries; want 3 (turbo, go/pkg/mod, pnpm-store)", len(reclaimed))
	}
	if len(skipped) != 3 {
		t.Errorf("skipped = %d entries; want 3 (go-build, grype, trivy)", len(skipped))
	}
}

// TestDiskCleanupWithOptions_ProtectedCachesSurvivePressureEscalation exercises the
// same protection through the top-level entry point, with disk pressure forcing the
// shared-cache tier to run even though a runner is busy — protectedCacheSubpaths must
// still be excluded "at every tier", pressure included.
func TestDiskCleanupWithOptions_ProtectedCachesSurvivePressureEscalation(t *testing.T) {
	withoutStalenessGuard(t)
	home := t.TempDir()
	mustMkdirWithFile(t, filepath.Join(home, ".cache", "go-build"), "compiled object")
	mustMkdirWithFile(t, filepath.Join(home, ".cache", "grype"), "vuln db")
	mustMkdirWithFile(t, filepath.Join(home, "go", "pkg", "mod"), "downloaded module")

	busyRoot := t.TempDir()
	withBusyRunner(t, busyRoot)
	withStubbedSideEffects(t)

	result := DiskCleanupWithOptions(DiskCleanupOptions{
		Home:              home,
		RunnerRoots:       []RunnerRoot{{Name: "busy-runner", Path: busyRoot}},
		PressureThreshold: 1,
		UsageOverride:     &DiskUsage{UsedPercent: 90}, // deterministic pressure trigger
		SharedCacheRoots:  []string{},
	})

	if exists(t, filepath.Join(home, "go", "pkg", "mod")) {
		t.Error("expected regenerable cache (go/pkg/mod) to be reclaimed under pressure despite busy runner")
	}
	for _, protected := range []string{"go-build", "grype"} {
		if !exists(t, filepath.Join(home, ".cache", protected)) {
			t.Errorf("protected cache %s must survive even under pressure escalation", protected)
		}
	}
	_ = result
}

// ── per-runner busy detection: clean the idle one, skip the busy one ───────────────

func TestPerRunnerBusyDetection_CleansIdleSkipsBusy(t *testing.T) {
	withoutStalenessGuard(t)
	idleRoot := t.TempDir()
	idleJobDir := filepath.Join(idleRoot, "_work", "nself-org", "cli")
	mustMkdirWithFile(t, idleJobDir, "idle runner's stale job checkout")

	busyRoot := t.TempDir()
	busyJobDir := filepath.Join(busyRoot, "_work", "nself-org", "ntask")
	mustMkdirWithFile(t, busyJobDir, "busy runner's in-progress job checkout")

	withBusyRunner(t, busyRoot)
	withStubbedSideEffects(t)

	result := DiskCleanupWithOptions(DiskCleanupOptions{
		Home: t.TempDir(), // isolated, empty — caches tier is irrelevant to this test
		RunnerRoots: []RunnerRoot{
			{Name: "idle-runner", Path: idleRoot},
			{Name: "busy-runner", Path: busyRoot},
		},
		PressureThreshold: 100, // deliberately unreachable, isolates the busy-gate behavior
		UsageOverride:     &DiskUsage{UsedPercent: 50},
		SharedCacheRoots:  []string{},
	})

	if exists(t, idleJobDir) {
		t.Error("idle runner's job workspace should have been reclaimed")
	}
	if !exists(t, busyJobDir) {
		t.Error("busy runner's job workspace must be left alone")
	}

	foundBusySkip := false
	for _, s := range result.Skipped {
		if s.Path == busyRoot {
			foundBusySkip = true
			if !strings.Contains(s.Reason, "busy") {
				t.Errorf("skip reason for busy root = %q; want it to mention busy", s.Reason)
			}
		}
	}
	if !foundBusySkip {
		t.Error("expected a Skipped entry explaining why the busy runner root was left alone")
	}
	if result.BytesReclaimed == 0 {
		t.Error("expected non-zero bytes reclaimed from the idle runner's job workspace")
	}
}

// ── pressure escalation triggers above threshold ────────────────────────────────────

func TestPressureEscalation_TriggersAboveThreshold(t *testing.T) {
	withoutStalenessGuard(t)
	home := t.TempDir()
	mustMkdirWithFile(t, filepath.Join(home, "pnpm-store"), "content-addressable store")

	busyRoot := t.TempDir()
	withBusyRunner(t, busyRoot)
	withStubbedSideEffects(t)

	// Below threshold: busy runner blocks the shared-cache tier entirely.
	belowResult := DiskCleanupWithOptions(DiskCleanupOptions{
		Home:              home,
		RunnerRoots:       []RunnerRoot{{Name: "busy-runner", Path: busyRoot}},
		PressureThreshold: 100,
		UsageOverride:     &DiskUsage{UsedPercent: 50},
		SharedCacheRoots:  []string{},
	})
	if !exists(t, filepath.Join(home, "pnpm-store")) {
		t.Fatal("pnpm-store should NOT have been reclaimed below the pressure threshold while busy")
	}
	foundCacheSkip := false
	for _, s := range belowResult.Skipped {
		if strings.Contains(s.Reason, "pressure threshold") {
			foundCacheSkip = true
		}
	}
	if !foundCacheSkip {
		t.Error("expected a Skipped entry citing the pressure threshold when below it")
	}

	// At/above threshold: shared caches reclaim regardless of the busy runner.
	aboveResult := DiskCleanupWithOptions(DiskCleanupOptions{
		Home:              home,
		RunnerRoots:       []RunnerRoot{{Name: "busy-runner", Path: busyRoot}},
		PressureThreshold: 85,
		UsageOverride:     &DiskUsage{UsedPercent: 90},
		SharedCacheRoots:  []string{},
	})
	if exists(t, filepath.Join(home, "pnpm-store")) {
		t.Error("pnpm-store should have been reclaimed once disk usage crossed the pressure threshold")
	}
	if aboveResult.BytesReclaimed == 0 {
		t.Error("expected non-zero bytes reclaimed once pressure escalation kicked in")
	}
}

// ── dry-run removes nothing ─────────────────────────────────────────────────────────

func TestDiskCleanupWithOptions_DryRunRemovesNothing(t *testing.T) {
	withoutStalenessGuard(t)
	home := t.TempDir()
	mustMkdirWithFile(t, filepath.Join(home, ".cache", "turbo"), "turborepo cache")
	mustMkdirWithFile(t, filepath.Join(home, "go", "pkg", "mod"), "downloaded module")
	mustMkdirWithFile(t, filepath.Join(home, "pnpm-store"), "content-addressable store")

	runnerRoot := t.TempDir()
	jobDir := filepath.Join(runnerRoot, "_work", "nself-org", "cli")
	mustMkdirWithFile(t, jobDir, "job checkout")

	withNoBusyRunners(t)
	withStubbedSideEffects(t)

	result := DiskCleanupWithOptions(DiskCleanupOptions{
		DryRun:            true,
		Home:              home,
		RunnerRoots:       []RunnerRoot{{Name: "idle-runner", Path: runnerRoot}},
		PressureThreshold: 100,
		UsageOverride:     &DiskUsage{UsedPercent: 50},
		SharedCacheRoots:  []string{},
	})

	for _, p := range []string{
		filepath.Join(home, ".cache", "turbo"),
		filepath.Join(home, "go", "pkg", "mod"),
		filepath.Join(home, "pnpm-store"),
		jobDir,
	} {
		if !exists(t, p) {
			t.Errorf("dry-run removed %s; it must not remove anything", p)
		}
	}

	if !result.DryRun {
		t.Error("CleanupResult.DryRun should be true")
	}
	if len(result.Reclaimed) == 0 {
		t.Error("dry-run should still report what WOULD have been reclaimed")
	}
	for _, r := range result.Reclaimed {
		if r.Bytes == 0 {
			t.Errorf("dry-run reclaim entry %s reports 0 bytes; want the real would-be size", r.Path)
		}
	}
}

// ── filterAnonymousVolumes: never a named "*_data" volume ─────────────────────────

func TestFilterAnonymousVolumes_ExcludesNamedVolumes(t *testing.T) {
	names := []string{
		"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", // 64 hex chars
		"ntask_data",
		"hasura_data",
		"",
		"not-hex-and-not-64-chars",
	}
	got := filterAnonymousVolumes(names)
	if len(got) != 1 || got[0] != names[0] {
		t.Errorf("filterAnonymousVolumes(%v) = %v; want only the 64-hex anonymous name", names, got)
	}
}

// TestReclaimRunnerWork_SkipsRecentlyModifiedWorkspace covers the race that a
// busy-process check cannot: a runner reports idle, and a job starts before the
// RemoveAll lands. On 2026-09-11 that destroyed three live jobs on nSelf
// staging ("Directory .../_work/web/web does not exist"). A live job writes
// into its workspace constantly, so a recent mtime must veto removal even when
// the runner looks idle.
func TestReclaimRunnerWork_SkipsRecentlyModifiedWorkspace(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "_work")

	active := filepath.Join(work, "active-repo", "active-repo")
	if err := os.MkdirAll(active, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file written now stands in for a job mid-build.
	if err := os.WriteFile(filepath.Join(active, "building.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	stale := filepath.Join(work, "abandoned-repo", "abandoned-repo")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	staleFile := filepath.Join(stale, "old.log")
	if err := os.WriteFile(staleFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Age the whole abandoned tree well past the staleness window.
	old := time.Now().Add(-2 * workspaceStaleAfter)
	for _, p := range []string{staleFile, stale, filepath.Join(work, "abandoned-repo")} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	_, reclaimed, skipped := reclaimRunnerWork(root, false)

	if _, err := os.Stat(filepath.Join(work, "active-repo")); err != nil {
		t.Fatalf("active workspace was removed despite a recent write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "abandoned-repo")); !os.IsNotExist(err) {
		t.Fatalf("abandoned workspace should have been reclaimed, stat err = %v", err)
	}

	var skippedActive bool
	for _, s := range skipped {
		if strings.Contains(s.Path, "active-repo") && strings.Contains(s.Reason, "active job workspace") {
			skippedActive = true
		}
	}
	if !skippedActive {
		t.Errorf("expected active-repo to be skipped as an active workspace, got skips: %+v", skipped)
	}

	var reclaimedStale bool
	for _, r := range reclaimed {
		if strings.Contains(r.Path, "abandoned-repo") {
			reclaimedStale = true
		}
	}
	if !reclaimedStale {
		t.Errorf("expected abandoned-repo to be reclaimed, got: %+v", reclaimed)
	}
}

// TestPressureThresholdStaysBelowDoctorHostLimit pins the relationship between
// this package's escalation point and the doctor host-disk check. doctor fails
// at 80% used (<20% free); if escalation were at or above that, the box could
// sit in a band where cleanup is satisfied and doctor is red — the exact state
// nSelf staging was in on 2026-09-11.
func TestPressureThresholdStaysBelowDoctorHostLimit(t *testing.T) {
	const doctorHostDiskUsedLimit = 80
	if DefaultPressureThreshold >= doctorHostDiskUsedLimit {
		t.Fatalf("DefaultPressureThreshold (%d) must stay below the doctor host-disk limit (%d%% used); "+
			"otherwise cleanup never escalates in the band where doctor already fails",
			DefaultPressureThreshold, doctorHostDiskUsedLimit)
	}
}
