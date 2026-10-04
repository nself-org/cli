package repoqa

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// requireTool resolves an external tool a repoqa test needs.
//
// Purpose:     one rule for missing tools (EPIC G5). Locally a missing tool
//
//	skips the test; when CI is set it fails, so a runner that lost
//	`go` or `git` cannot turn a guard into a silent no-op.
//
// Inputs:      tb, the test (or a fake in TestRequireTool); name, the binary
//
//	to find on PATH.
//
// Outputs:     the resolved path. When the tool is missing it does not return:
//
//	it ends the test through tb.Fatalf (CI set) or tb.Skipf (CI unset).
func requireTool(tb testing.TB, name string) string {
	tb.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("CI") != "" {
			tb.Fatalf("%s not on PATH (required in CI)", name)
		} else {
			tb.Skipf("%s not on PATH", name)
		}
		return ""
	}
	return path
}

// fakeTB records the verdict of a call that would end a test. Fatalf and Skipf
// stop the calling goroutine with runtime.Goexit, as the real testing.TB does,
// so requireTool must run in its own goroutine (see runRequireTool).
type fakeTB struct {
	testing.TB
	fatal, skip string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatal = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func (f *fakeTB) Skipf(format string, args ...any) {
	f.skip = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// runRequireTool calls requireTool against a fake TB in a fresh goroutine and
// returns the path it produced plus the fake's recorded verdict.
func runRequireTool(name string) (path string, f *fakeTB) {
	f = &fakeTB{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		path = requireTool(f, name)
	}()
	wg.Wait()
	return path, f
}

// TestRequireTool covers the three outcomes: tool present, tool missing with
// CI set (fatal), tool missing with CI unset (skip).
func TestRequireTool(t *testing.T) {
	const missing = "nself-repoqa-no-such-tool"
	cases := []struct {
		name      string
		tool      string
		ci        string
		wantPath  bool
		wantFatal bool
		wantSkip  bool
	}{
		{"present tool, CI unset", "go", "", true, false, false},
		{"present tool, CI set", "go", "true", true, false, false},
		{"missing tool, CI set", missing, "true", false, true, false},
		{"missing tool, CI unset", missing, "", false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPath {
				if _, err := exec.LookPath(tc.tool); err != nil {
					t.Skipf("%s not on PATH", tc.tool)
				}
			}
			t.Setenv("CI", tc.ci)
			path, f := runRequireTool(tc.tool)
			if gotPath := path != ""; gotPath != tc.wantPath {
				t.Errorf("path = %q, want non-empty=%v", path, tc.wantPath)
			}
			if gotFatal := f.fatal != ""; gotFatal != tc.wantFatal {
				t.Errorf("fatal = %q, want fatal=%v", f.fatal, tc.wantFatal)
			}
			if gotSkip := f.skip != ""; gotSkip != tc.wantSkip {
				t.Errorf("skip = %q, want skip=%v", f.skip, tc.wantSkip)
			}
			if tc.wantFatal && !strings.Contains(f.fatal, "required in CI") {
				t.Errorf("fatal message %q does not say the tool is required in CI", f.fatal)
			}
		})
	}
}
