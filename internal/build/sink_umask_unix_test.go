//go:build unix

package build

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDiskSinkWriteFileUnderRestrictiveUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	dir := t.TempDir()
	sink := newDiskSink(dir)
	for _, mode := range []os.FileMode{0o644, 0o600, 0o755} {
		path := filepath.Join(dir, mode.String())
		if err := sink.WriteFile(path, []byte("first"), mode); err != nil {
			t.Fatal(err)
		}
		assertArtifact(t, path, "first", mode)
		// Rewrites retain the existing mode, as the planner does.
		if err := sink.WriteFile(path, []byte("second"), 0o666); err != nil {
			t.Fatal(err)
		}
		assertArtifact(t, path, "second", mode)
	}
}

func TestBuildPlannedModesUnderRestrictiveUmask(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	plan, err := Build(f.workdir, BuildOptions{Mode: ModePlan})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(f.workdir, BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	for key, want := range plan.Planned.Files {
		info, err := os.Stat(plannedDiskPath(f, key))
		if err != nil {
			t.Errorf("%s: %v", key, err)
			continue
		}
		if got := info.Mode().Perm(); got != want.Perm {
			t.Errorf("%s: mode %04o, planned %04o", key, got, want.Perm)
		}
	}
}
