package dockertest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStrict(t *testing.T) {
	for _, tc := range []struct {
		goos, ga string
		want     bool
	}{
		{"linux", "true", true},
		{"linux", "", false},  // cam CI host (CI=1, no GITHUB_ACTIONS)
		{"linux", "1", false}, // only the exact value GitHub sets counts
		{"darwin", "true", false},
		{"windows", "true", false},
	} {
		if got := Strict(tc.goos, tc.ga); got != tc.want {
			t.Errorf("Strict(%q, %q) = %v, want %v", tc.goos, tc.ga, got, tc.want)
		}
	}
}

func TestReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		out    string
		err    error
		usable bool
	}{
		{"linux daemon", "linux", nil, true},
		{"socket permission denied", "permission denied while trying to connect", errors.New("exit status 1"), false},
		{"windows containers", "windows", nil, false},
		{"empty answer", "", nil, false},
	} {
		if got := reason(tc.out, tc.err); (got == "") != tc.usable {
			t.Errorf("%s: reason = %q, want usable=%v", tc.name, got, tc.usable)
		}
	}
}

// fakeTB records Skipf and Fatalf instead of ending the test.
type fakeTB struct {
	testing.TB
	skipped, failed string
}

func (f *fakeTB) Helper()                           {}
func (f *fakeTB) Skipf(format string, args ...any)  { f.skipped = fmt.Sprintf(format, args...) }
func (f *fakeTB) Fatalf(format string, args ...any) { f.failed = fmt.Sprintf(format, args...) }

func TestRequire(t *testing.T) {
	for _, tc := range []struct {
		name, why, ga      string
		wantSkip, wantFail bool
	}{
		{"usable daemon", "", "true", false, false},
		{"unusable, not GitHub", "permission denied", "", true, false},
		{"unusable, GitHub", "permission denied", "true", runtime.GOOS != "linux", runtime.GOOS == "linux"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := probeDocker
			probeDocker = func() string { return tc.why }
			t.Cleanup(func() { probeDocker = old })
			t.Setenv("GITHUB_ACTIONS", tc.ga)
			f := &fakeTB{TB: t}
			Require(f)
			if (f.skipped != "") != tc.wantSkip || (f.failed != "") != tc.wantFail {
				t.Fatalf("skipped %q failed %q, want skip=%v fail=%v", f.skipped, f.failed, tc.wantSkip, tc.wantFail)
			}
			if tc.why != "" && !strings.Contains(f.skipped+f.failed, tc.why) {
				t.Fatalf("reason %q not reported: skipped %q failed %q", tc.why, f.skipped, f.failed)
			}
		})
	}
}

func TestProbe(t *testing.T) {
	t.Run("no docker CLI", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if why := probe(); !strings.Contains(why, "docker CLI not found") {
			t.Fatalf("probe = %q", why)
		}
	})
	if runtime.GOOS == "windows" {
		return // the stub below is a shell script
	}
	for _, tc := range []struct{ name, script, want string }{
		{"linux daemon", "echo linux", ""},
		{"socket refused", "echo 'permission denied' >&2; exit 1", "permission denied"},
		{"windows daemon", "echo windows", "OS type is windows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n"+tc.script+"\n"), 0o755); err != nil { //nolint:gosec // test stub must be executable
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			why := probe()
			if (tc.want == "" && why != "") || !strings.Contains(why, tc.want) {
				t.Fatalf("probe = %q, want %q", why, tc.want)
			}
		})
	}
}

func TestRuntimeFailureClassifies(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Setenv("GITHUB_ACTIONS", "")
	}
	infra := &fakeTB{TB: t}
	if !RuntimeFailure(infra, errors.New("docker: Error response from daemon: failed to create task for container: failed to create shim task"), "") || infra.skipped == "" {
		t.Fatalf("an infrastructure error must skip: %+v", infra)
	}
	own := &fakeTB{TB: t}
	if RuntimeFailure(own, errors.New("exit status 1"), "PROBE-FAIL: wrong header") || own.skipped != "" || own.failed != "" {
		t.Fatalf("a test's own failure must not be classified as infrastructure: %+v", own)
	}
	if RuntimeFailure(&fakeTB{TB: t}, nil, "") {
		t.Fatal("nil error is never a runtime failure")
	}
}

func TestRuntimeFailureStrictOnGitHubLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Strict is linux-only")
	}
	t.Setenv("GITHUB_ACTIONS", "true")
	f := &fakeTB{TB: t}
	RuntimeFailure(f, errors.New("failed to create shim task"), "")
	if f.failed == "" || f.skipped != "" {
		t.Fatalf("on a GitHub Linux runner an infrastructure error must fail: %+v", f)
	}
}
