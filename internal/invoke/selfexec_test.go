package invoke

// Purpose: prove NSELF_MCP_EXEC_OVERRIDE is honoured only in a `go test`
// binary and ignored by a release build (D-0288).
// Constraints: the release-build case compiles a probe through a -overlay so
// the source tree stays untouched; it is skipped in -short mode.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const fakeOverride = "/opt/fake-nself-d0288"

func TestSelfExecutableOverrideInTestBinary(t *testing.T) {
	if !selfExecOverrideAllowed {
		t.Fatal("a go test binary must allow the override")
	}
	t.Setenv(SelfExecOverrideEnv, fakeOverride)
	got, err := SelfExecutable()
	if err != nil || got != fakeOverride {
		t.Fatalf("test binary: got %q, %v; want the override", got, err)
	}
}

func TestSelfExecutableReleasePathIgnoresEnv(t *testing.T) {
	saved := selfExecOverrideAllowed
	selfExecOverrideAllowed = false
	t.Cleanup(func() { selfExecOverrideAllowed = saved })
	t.Setenv(SelfExecOverrideEnv, fakeOverride)
	want, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	got, err := SelfExecutable()
	if err != nil || got != want {
		t.Fatalf("release path: got %q, %v; want os.Executable() %q", got, err, want)
	}
}

const releaseProbe = `package main

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/invoke"
)

func main() {
	got, err := invoke.SelfExecutable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(got)
}
`

func TestSelfExecutableReleaseBuildIgnoresEnv(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the release-build probe in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "main.go")
	if err := os.WriteFile(src, []byte(releaseProbe), 0o600); err != nil {
		t.Fatal(err)
	}
	// The probe package exists only in the overlay, never on disk.
	virtual := filepath.Join(pkgDir, "selfexecprobe_d0288", "main.go")
	overlay, err := json.Marshal(map[string]map[string]string{"Replace": {virtual: src}})
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(tmp, "overlay.json")
	if err := os.WriteFile(overlayPath, overlay, 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "probe")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command(goBin, "build", "-overlay", overlayPath, "-o", bin, "./selfexecprobe_d0288")
	build.Dir = pkgDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the release probe: %v\n%s", err, out)
	}

	run := exec.Command(bin)
	run.Env = append(os.Environ(), SelfExecOverrideEnv+"="+fakeOverride)
	out, err := run.Output()
	if err != nil {
		t.Fatalf("running the release probe: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got == fakeOverride {
		t.Fatalf("a release build honoured %s=%s", SelfExecOverrideEnv, fakeOverride)
	}
	gotReal, err1 := filepath.EvalSymlinks(got)
	wantReal, err2 := filepath.EvalSymlinks(bin)
	if err1 != nil || err2 != nil || gotReal != wantReal {
		t.Fatalf("release build resolved %q, want its own executable %q (%v, %v)", got, bin, err1, err2)
	}
}
