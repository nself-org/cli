package backup

// Purpose: pin C2-03: restore-remote streams into a scratch database and swaps
// it in (D-0663), leaves the live database alone on failure, and needs no host
// pg_restore (it runs inside the postgres container).
// Inputs: the fakeAtomicRestoreDocker stub, a path:// object, a PATH holding
// only docker and cat. Outputs: test assertions only.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// remoteRestoreFixture returns a path:// URI for a dump object and the fake
// docker's target-state and log files. PATH holds only docker and cat, so
// neither rclone nor a host pg_restore is reachable.
func remoteRestoreFixture(t *testing.T) (from, target, log string) {
	t.Helper()
	dir := t.TempDir()
	target, log = filepath.Join(dir, "target-state"), filepath.Join(dir, "docker.log")
	if err := os.WriteFile(target, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	objects := t.TempDir()
	if err := os.WriteFile(filepath.Join(objects, "p_full_1.dump"), []byte("PGDMP"), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat not on PATH")
	}
	fakeAtomicRestoreDocker(t, target, log)
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Dir(dockerPath)
	if err := os.Symlink(cat, filepath.Join(bin, "cat")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return "path://" + objects + "/p_full_1.dump", target, log
}

func remoteRestoreConfig() *config.Config {
	cfg := &config.Config{ProjectName: "p"}
	cfg.Postgres.DB = "live"
	return cfg
}

func TestRestoreFromRemoteSwapsScratchDatabase(t *testing.T) {
	from, target, log := remoteRestoreFixture(t)
	if err := RestoreFromRemote(t.Context(), remoteRestoreConfig(), from, ""); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "restored\n" {
		t.Fatalf("target state = %q, want restored", got)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	text := string(calls)
	if strings.Contains(text, "--clean") || strings.Contains(text, " -d live ") {
		t.Fatalf("restore-remote touched the live database in place:\n%s", text)
	}
	rest := text
	for _, want := range []string{
		"CREATE DATABASE \"nself_restore_",
		"pg_restore -U postgres -d nself_restore_",
		"BEGIN; ALTER DATABASE \"live\" RENAME TO \"nself_previous_",
		"DROP DATABASE \"nself_previous_",
	} {
		i := strings.Index(rest, want)
		if i < 0 {
			t.Fatalf("missing %q in order, calls:\n%s", want, text)
		}
		rest = rest[i+len(want):]
	}
	if err := RestoreFromRemote(t.Context(), remoteRestoreConfig(), "path://"+filepath.Dir(strings.TrimPrefix(from, "path://"))+"/missing.dump", ""); err == nil {
		t.Fatal("missing object restored")
	}
}

// TestRestoreFromRemoteFailureLeavesLiveUntouched pins the strict rule in both
// compat modes: success swaps the scratch database in and drops the previous
// one, so a pg_restore that exits non-zero (with the COPY error line v1.4 used
// to accept, or with no output at all) must fail before the swap.
func TestRestoreFromRemoteFailureLeavesLiveUntouched(t *testing.T) {
	for _, tc := range []struct {
		name  string
		v15   bool
		stage string
	}{
		{"v1.4 COPY error line", false, "restore"},
		{"v1.5 COPY error line", true, "restore"},
		{"v1.4 silent non-zero exit", false, "restore_quiet"},
		{"v1.5 silent non-zero exit", true, "restore_quiet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			from, target, log := remoteRestoreFixture(t)
			t.Setenv("FAKE_FAIL_STAGE", tc.stage)
			err := RestoreFromRemote(t.Context(), remoteRestoreConfig(), from, "")
			if err == nil || !errors.Is(err, errs.ErrBackupRestoreFailed) {
				t.Fatalf("a failed scratch restore must fail, got %v", err)
			}
			if got, _ := os.ReadFile(target); string(got) != "original\n" {
				t.Fatalf("failed restore changed the live database: %q", got)
			}
			calls, _ := os.ReadFile(log)
			text := string(calls)
			if strings.Contains(text, "RENAME TO") || strings.Contains(text, "DROP DATABASE \"nself_previous_") {
				t.Fatalf("want no swap and no drop of the live database, calls:\n%s", text)
			}
			if !strings.Contains(text, "--exit-on-error") || !strings.Contains(text, "DROP DATABASE \"nself_restore_") {
				t.Fatalf("want pg_restore --exit-on-error and the scratch database dropped, calls:\n%s", text)
			}
		})
	}
}
