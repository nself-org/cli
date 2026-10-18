//go:build darwin || linux

package maintenance

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/fslock"
)

// D-0947: every crontab read-modify-write holds the per-user lock, so two builds of different
// projects cannot lose each other's line.
func TestCrontabChangesHoldTheUserLock(t *testing.T) {
	f, root := newFake(t, "linux", 1000)
	dir, bin := project(t, root)
	f.exe = bin
	lock := filepath.Join(root, ".nself", "entitlement-timer.lock")
	var unlocked []string // crontab calls that ran while nobody held the lock
	inner := f.run
	f.host.run = func(stdin, name string, args ...string) (string, error) {
		if name == "crontab" {
			if release, err := fslock.Acquire(lock, 20*time.Millisecond); err == nil {
				release()
				unlocked = append(unlocked, "crontab "+strings.Join(args, " "))
			} else if !errors.Is(err, fslock.ErrTimeout) {
				t.Fatal(err)
			}
		}
		return inner(stdin, name, args...)
	}
	if _, err := f.ensure(dir, bin); err != nil {
		t.Fatal(err)
	}
	if _, err := f.remove(Hash(dir)); err != nil {
		t.Fatal(err)
	}
	if len(unlocked) > 0 {
		t.Fatalf("crontab ran without the user lock: %v", unlocked)
	}
}

// D-0947: a second writer waits for the first instead of reading a stale crontab.
func TestEnsureWaitsForTheUserLock(t *testing.T) {
	f, root := newFake(t, "linux", 1000)
	dir, bin := project(t, root)
	f.exe = bin
	release, err := fslock.Acquire(filepath.Join(root, ".nself", "entitlement-timer.lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := f.ensure(dir, bin); done <- err }()
	select {
	case err := <-done:
		t.Fatalf("ensure finished while the lock was held: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.cron, entCronTag+Hash(dir)) {
		t.Fatalf("crontab line missing after the lock was released: %q", f.cron)
	}
}

// D-0947: "binary gone" means absent; a binary that is briefly not executable (an upgrade in
// progress) must not make the timer remove itself.
func TestGuardTreatsOnlyAMissingBinaryAsGone(t *testing.T) {
	_, root := newFake(t, "linux", 1000)
	dir, bin := project(t, root)
	if err := exec.Command("chmod", "644", bin).Run(); err != nil {
		t.Fatal(err)
	}
	run := func(script string) int {
		err := exec.Command("sh", "-c", script).Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		return 0
	}
	gone := "exit 78"
	if got := run(refreshScript(bin, dir, gone) + "; exec " + shQuote(bin)); got == ExitGone {
		t.Fatalf("non-executable binary was treated as gone (exit %d)", got)
	}
	if got := run(refreshScript(bin+".missing", dir, gone) + "; exec true"); got != ExitGone {
		t.Fatalf("missing binary exit = %d, want %d", got, ExitGone)
	}
	for _, text := range []string{renderAllText(t, bin, dir)} {
		if strings.Contains(text, "test -x") {
			t.Fatalf("rendered text still guards with test -x:\n%s", text)
		}
	}
}

func renderAllText(t *testing.T, bin, dir string) string {
	t.Helper()
	out, err := renderAll(bin, dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
