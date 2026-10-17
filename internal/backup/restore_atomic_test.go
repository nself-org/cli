package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

func fakeAtomicRestoreDocker(t *testing.T, target, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("docker stub is a shell script")
	}
	bin := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_DOCKER_LOG"
case "$*" in
  *pg_restore*)
    case "$*" in
      *--clean*|*" -d live "*)
        cat >/dev/null
        printf 'half-applied\n' > "$FAKE_TARGET_STATE"
        echo 'pg_restore: error: cannot drop auth.users' >&2
        exit 1
        ;;
    esac
    if [ "$FAKE_FAIL_STAGE" = cancel ]; then
      : > "$FAKE_RESTORE_STARTED"
      exec sleep 20
    fi
    cat >/dev/null
    if [ "$FAKE_FAIL_STAGE" = restore ]; then
      echo 'pg_restore: error: COPY failed for table users' >&2
      exit 1
    fi
    exit 0
    ;;
  *"CREATE DATABASE "*)
    [ "$FAKE_FAIL_STAGE" = create ] && echo 'create failed' >&2 && exit 1
    ;;
  *"ALLOW_CONNECTIONS false"*)
    [ "$FAKE_FAIL_STAGE" = block ] && echo 'block failed' >&2 && exit 1
    ;;
  *pg_terminate_backend*)
    [ "$FAKE_FAIL_STAGE" = terminate ] && echo 'terminate failed' >&2 && exit 1
    ;;
  *"BEGIN; ALTER DATABASE \"live\" RENAME TO \"nself_previous_"*)
    { [ "$FAKE_FAIL_STAGE" = swap ] || [ "$FAKE_FAIL_STAGE" = swap_and_unblock ]; } && echo 'rename failed' >&2 && exit 1
    printf 'restored\n' > "$FAKE_TARGET_STATE"
    ;;
  *"ALLOW_CONNECTIONS true"*)
    [ "$FAKE_FAIL_STAGE" = swap_and_unblock ] && echo 'unblock failed' >&2 && exit 1
    ;;
  *"DROP DATABASE \"nself_previous_"*)
    [ "$FAKE_FAIL_STAGE" = cleanup ] && echo 'drop failed' >&2 && exit 1
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_LOG", log)
	t.Setenv("FAKE_TARGET_STATE", target)
}

func atomicRestoreFixture(t *testing.T) (dump, target, log string) {
	t.Helper()
	dir := t.TempDir()
	target = filepath.Join(dir, "target-state")
	log = filepath.Join(dir, "docker.log")
	if err := os.WriteFile(target, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dump = filepath.Join(dir, "backup.dump")
	if err := os.WriteFile(dump, []byte("PGDMP"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeAtomicRestoreDocker(t, target, log)
	return dump, target, log
}

// TestRestorePgDumpFailureLeavesTargetUntouched reproduces D-0663: restoring
// over a migrated database must fail in scratch space before changing live.
func TestRestorePgDumpFailureLeavesTargetUntouched(t *testing.T) {
	compattest.Set(t, true)
	dump, target, log := atomicRestoreFixture(t)
	t.Setenv("FAKE_FAIL_STAGE", "restore")
	err := restorePgDump(context.Background(), "container", "postgres", "live", dump, false)
	if err == nil || !errors.Is(err, errs.ErrBackupRestoreFailed) {
		t.Fatalf("scratch restore failure must fail safely, got %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "original\n" {
		t.Fatalf("failed restore changed target: state=%q err=%v", got, err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if text := string(calls); strings.Contains(text, "--clean") || !strings.Contains(text, "--exit-on-error") {
		t.Fatalf("restore must use scratch-safe pg_restore flags, calls:\n%s", text)
	}
}

func TestRestorePgDumpSwapsOnlyAfterScratchRestore(t *testing.T) {
	dump, target, log := atomicRestoreFixture(t)
	if err := restorePgDump(context.Background(), "container", "postgres", "live", dump, false); err != nil {
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
	wants := []string{
		"CREATE DATABASE \"nself_restore_",
		"WITH TEMPLATE template0 OWNER \"postgres\"",
		"pg_restore -U postgres -d nself_restore_",
		"--exit-on-error",
		"ALTER DATABASE \"live\" WITH ALLOW_CONNECTIONS false",
		"pg_terminate_backend(pid, 5000)",
		"BEGIN; ALTER DATABASE \"live\" RENAME TO \"nself_previous_",
		"ALTER DATABASE \"nself_restore_",
		"RENAME TO \"live\"; ALTER DATABASE \"live\" WITH ALLOW_CONNECTIONS true; COMMIT",
		"DROP DATABASE \"nself_previous_",
	}
	rest := text
	for _, want := range wants {
		at := strings.Index(rest, want)
		if at < 0 {
			t.Errorf("missing %q in calls:\n%s", want, text)
			continue
		}
		rest = rest[at+len(want):]
	}
	if strings.Contains(text, "--clean") {
		t.Fatalf("scratch restore must not use --clean:\n%s", text)
	}
}

func TestRestorePgDumpPreSwapFailuresLeaveTargetUntouched(t *testing.T) {
	for _, stage := range []string{"create", "restore", "block", "terminate", "swap"} {
		t.Run(stage, func(t *testing.T) {
			dump, target, log := atomicRestoreFixture(t)
			t.Setenv("FAKE_FAIL_STAGE", stage)
			err := restorePgDump(context.Background(), "container", "postgres", "live", dump, false)
			if err == nil || !errors.Is(err, errs.ErrBackupRestoreFailed) {
				t.Fatalf("%s failure = %v, want restore error", stage, err)
			}
			if got, _ := os.ReadFile(target); string(got) != "original\n" {
				t.Fatalf("%s failure changed target to %q", stage, got)
			}
			calls, _ := os.ReadFile(log)
			if stage != "swap" && strings.Contains(string(calls), "BEGIN; ALTER DATABASE \"live\" RENAME") {
				t.Fatalf("%s failure reached database swap:\n%s", stage, calls)
			}
			if stage == "swap" && !strings.Contains(string(calls), "ALTER DATABASE \"live\" WITH ALLOW_CONNECTIONS true") {
				t.Fatalf("swap failure did not unblock original target:\n%s", calls)
			}
		})
	}
}

func TestRestorePgDumpCleanupFailureRetainsPreviousDatabase(t *testing.T) {
	dump, target, log := atomicRestoreFixture(t)
	t.Setenv("FAKE_FAIL_STAGE", "cleanup")
	err := restorePgDump(context.Background(), "container", "postgres", "live", dump, false)
	if err == nil || !strings.Contains(err.Error(), "previous database retained as nself_previous_") {
		t.Fatalf("cleanup failure must report retained database, got %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "restored\n" {
		t.Fatalf("cleanup failure changed restored target to %q", got)
	}
	calls, _ := os.ReadFile(log)
	if strings.Count(string(calls), "DROP DATABASE \"nself_previous_") != 1 {
		t.Fatalf("previous database cleanup was not attempted once:\n%s", calls)
	}
}

func TestRestorePgDumpReportsFailedSwapRecovery(t *testing.T) {
	dump, target, _ := atomicRestoreFixture(t)
	t.Setenv("FAKE_FAIL_STAGE", "swap_and_unblock")
	err := restorePgDump(context.Background(), "container", "postgres", "live", dump, false)
	if err == nil || !strings.Contains(err.Error(), "swap restored database") || !strings.Contains(err.Error(), "re-enable original database connections") {
		t.Fatalf("swap and recovery failure = %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "original\n" {
		t.Fatalf("failed swap recovery changed target to %q", got)
	}
}

func TestRestorePgDumpCancellationDropsScratchAndPreservesTarget(t *testing.T) {
	dump, target, log := atomicRestoreFixture(t)
	started := filepath.Join(t.TempDir(), "restore-started")
	t.Setenv("FAKE_FAIL_STAGE", "cancel")
	t.Setenv("FAKE_RESTORE_STARTED", started)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- restorePgDump(ctx, "container", "postgres", "live", dump, false) }()
	for deadline := time.Now().Add(3 * time.Second); ; {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pg_restore did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, errs.ErrBackupRestoreFailed) || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("cancelled restore = %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "original\n" {
		t.Fatalf("cancelled restore changed target to %q", got)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "DROP DATABASE \"nself_restore_") {
		t.Fatalf("cancelled restore left scratch database:\n%s", calls)
	}
}

func TestRestorePgDumpQuotesDatabaseAndRoleNames(t *testing.T) {
	dump, _, log := atomicRestoreFixture(t)
	if err := restorePgDump(context.Background(), "container", `role"name`, `live\'db`, dump, false); err != nil {
		t.Fatalf("restore: %v", err)
	}
	calls, _ := os.ReadFile(log)
	text := string(calls)
	if !strings.Contains(text, `OWNER "role""name"`) || !strings.Contains(text, `datname = E'live\\''db'`) {
		t.Fatalf("SQL names were not quoted safely:\n%s", text)
	}
}

func TestRestorePgDumpUsesTemplate1WhenReplacingPostgres(t *testing.T) {
	dump, _, log := atomicRestoreFixture(t)
	if err := restorePgDump(context.Background(), "container", "postgres", "postgres", dump, false); err != nil {
		t.Fatalf("restore postgres: %v", err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "-d template1 -c CREATE DATABASE") {
		t.Fatalf("postgres target must use template1 for maintenance:\n%s", calls)
	}
}

func TestRestorePgDumpRefusesTemplateDatabases(t *testing.T) {
	for _, db := range []string{"template0", "template1"} {
		t.Run(db, func(t *testing.T) {
			dump := filepath.Join(t.TempDir(), "backup.dump")
			if err := os.WriteFile(dump, []byte("PGDMP"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := restorePgDump(context.Background(), "container", "postgres", db, dump, false); err == nil || !strings.Contains(err.Error(), "refusing to replace PostgreSQL template") {
				t.Fatalf("template restore = %v", err)
			}
		})
	}
}
