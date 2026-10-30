//go:build !windows

package build

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/sysexec"
)

func TestBuildSinkPlannedModesUnderPrivateUmask(t *testing.T) {
	old := syscall.Umask(0077)
	defer syscall.Umask(old)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".nself"), 0700); err != nil {
		t.Fatal(err)
	}
	plan, disk := newMemSink(dir, ""), newDiskSink(dir)
	for _, file := range []struct {
		name string
		mode os.FileMode
	}{
		{buildProfileFile, 0644},
		{".env.secrets", 0600},
		{"script.sh", 0755},
	} {
		for _, sink := range []Sink{plan, disk} {
			if err := sink.WriteFile(file.name, []byte("planned"), file.mode); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := newExpectSink(dir, plan.snapshot()).verifyPublished(); err != nil {
		t.Fatalf("umask changed the planned artifact modes: %v", err)
	}
	// Rewriting an existing private file must not widen its permissions.
	if err := disk.WriteFile(".env.secrets", []byte("updated"), 0644); err != nil {
		t.Fatal(err)
	}
	assertArtifact(t, filepath.Join(dir, ".env.secrets"), "updated", 0600)
}

func TestBuildUnderPrivateUmask(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		v15 := compat.V15()
		f := newPlanFixture(t, "dev-minimal")
		compattest.Set(t, v15) // fixture environment isolation also clears this key
		t.Cleanup(sysexec.Set(&sysexec.Fake{}))
		t.Setenv("PATH", t.TempDir())
		old := syscall.Umask(0077)
		defer syscall.Umask(old)
		if _, err := Build(f.workdir, BuildOptions{}); err != nil {
			t.Fatalf("build under umask 077: %v", err)
		}
		info, err := os.Stat(filepath.Join(f.workdir, buildProfileFile))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0644 {
			t.Fatalf("build profile mode = %04o, want 0644", info.Mode().Perm())
		}
	})
}
