package runner

import (
	"context"
	"errors"
	"testing"
)

func testManifest() *Manifest {
	return &Manifest{
		SchemaVersion: 1,
		RunnerUser:    "gha-runner",
		WorkDir:       "/opt/actions-runner/_work",
		Dependencies: []Dependency{
			{Name: "git", AptPackage: "git", Binary: "git", Reason: "checkout needs git"},
			{Name: "libssl-dev", AptPackage: "libssl-dev", Binary: "", Reason: "tls builds need headers"},
		},
		ChromiumCacheGlobs: []string{"%h/.cache/ms-playwright/chromium-*/chrome-linux/chrome"},
	}
}

func TestCheckDependency_BinaryPresent(t *testing.T) {
	ex := &fakeExecutor{}
	ex.when("command -v 'git'", "/usr/bin/git", nil)
	r := checkDependency(context.Background(), ex, Dependency{Name: "git", Binary: "git"})
	if r.Status != StatusPass {
		t.Fatalf("status = %v, want pass; detail=%s", r.Status, r.Detail)
	}
}

func TestCheckDependency_BinaryMissing(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "", err: errors.New("exit status 1")}}
	r := checkDependency(context.Background(), ex, Dependency{Name: "gh", Binary: "gh", Reason: "commit status posting"})
	if r.Status != StatusFail {
		t.Fatalf("status = %v, want fail", r.Status)
	}
	if r.Name != "dep:gh" {
		t.Fatalf("Name = %q", r.Name)
	}
}

func TestCheckDependency_AptPackageInstalled(t *testing.T) {
	ex := &fakeExecutor{}
	ex.when("dpkg -s 'libssl-dev'", "installed", nil)
	r := checkDependency(context.Background(), ex, Dependency{Name: "libssl-dev", AptPackage: "libssl-dev"})
	if r.Status != StatusPass {
		t.Fatalf("status = %v, want pass", r.Status)
	}
}

func TestCheckDependency_AptPackageMissing(t *testing.T) {
	ex := &fakeExecutor{}
	ex.when("dpkg -s 'libnspr4'", "missing", nil)
	r := checkDependency(context.Background(), ex, Dependency{Name: "libnspr4", AptPackage: "libnspr4", Reason: "chromium runtime"})
	if r.Status != StatusFail {
		t.Fatalf("status = %v, want fail; detail=%s", r.Status, r.Detail)
	}
}

func TestCheckWorkDirNotSymlink_Symlink(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "SYMLINK", err: nil}}
	r := checkWorkDirNotSymlink(context.Background(), ex, "/opt/actions-runner/_work")
	if r.Status != StatusFail {
		t.Fatalf("status = %v, want fail for symlinked work dir", r.Status)
	}
	if r.Name != "workdir:not-symlink" {
		t.Fatalf("Name = %q", r.Name)
	}
}

func TestCheckWorkDirNotSymlink_RealDir(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "DIR", err: nil}}
	r := checkWorkDirNotSymlink(context.Background(), ex, "/opt/actions-runner/_work")
	if r.Status != StatusPass {
		t.Fatalf("status = %v, want pass for real directory", r.Status)
	}
}

func TestCheckWorkDirNotSymlink_Absent(t *testing.T) {
	ex := &fakeExecutor{fallback: fakeResponse{out: "ABSENT", err: nil}}
	r := checkWorkDirNotSymlink(context.Background(), ex, "/opt/actions-runner/_work")
	if r.Status != StatusWarn {
		t.Fatalf("status = %v, want warn for not-yet-created dir", r.Status)
	}
}

func TestCheckWorkDirNotSymlink_Empty(t *testing.T) {
	r := checkWorkDirNotSymlink(context.Background(), &fakeExecutor{}, "")
	if r.Status != StatusWarn {
		t.Fatalf("status = %v, want warn when unconfigured", r.Status)
	}
}

func TestVerifyHost_RunsAllChecks(t *testing.T) {
	ex := &fakeExecutor{}
	ex.when("command -v 'git'", "/usr/bin/git", nil)
	ex.when("dpkg -s 'libssl-dev'", "installed", nil)
	ex.when("if [ -L", "DIR", nil)
	ex.when("shopt -s nullglob", "", nil)

	results := VerifyHost(context.Background(), ex, testManifest())
	// 2 dependencies + workdir + chromium-cache-warn = 4
	if len(results) != 4 {
		t.Fatalf("len(results) = %d, want 4: %+v", len(results), results)
	}
	for _, r := range results {
		if r.Status == StatusFail {
			t.Errorf("unexpected fail: %+v", r)
		}
	}
}

func TestShellQuote(t *testing.T) {
	got := shellQuote(`it's`)
	want := `'it'\''s'`
	if got != want {
		t.Fatalf("shellQuote = %q, want %q", got, want)
	}
}
