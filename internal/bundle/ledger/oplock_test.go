package ledger

// Operation lock tests (D-0322). The cross-process cases re-exec the test binary
// (TestOpLockHelperProcess) so a real second process holds the lock.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	envHelper  = "NSELF_OPLOCK_HELPER" // mode: hold-op | hold-ledger | probe
	envHelpDir = "NSELF_OPLOCK_DIR"    // the plugin directory
	envHelpCtl = "NSELF_OPLOCK_CTL"    // dir for the acquired / release marker files
)

// TestOpLockHelperProcess is the child: it does nothing unless envHelper is set.
func TestOpLockHelperProcess(t *testing.T) {
	mode := os.Getenv(envHelper)
	if mode == "" {
		return
	}
	root, ctl := os.Getenv(envHelpDir), os.Getenv(envHelpCtl)
	hold := func() {
		_ = os.WriteFile(filepath.Join(ctl, "acquired"), []byte("1"), 0o600)
		for i := 0; i < 1000; i++ { // max 10 s
			if _, err := os.Stat(filepath.Join(ctl, "release")); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	s := NewStore(root)
	switch mode {
	case "hold-op":
		if err := s.Operation(context.Background(), "child hold", func() error { hold(); return nil }); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
	case "hold-ledger":
		_ = os.MkdirAll(s.Dir(), 0o700)
		f, err := openLockFile(filepath.Join(s.Dir(), lockName))
		if err != nil {
			os.Exit(4)
		}
		unlock, err := lockFile(f, 5*time.Second)
		if err != nil {
			os.Exit(4)
		}
		hold()
		unlock()
	case "probe":
		opPollSlice = 20 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		err := s.Operation(ctx, "probe", func() error { return nil })
		switch {
		case err == nil:
		case errors.Is(err, context.DeadlineExceeded):
			os.Exit(3)
		default:
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
	}
	os.Exit(0)
}

// child is a running helper process.
type child struct {
	cmd *exec.Cmd
	ctl string
}

// startChild runs the helper in mode and, for the holder modes, waits until it
// holds the lock.
func startChild(t *testing.T, mode, root string) *child {
	t.Helper()
	needsFileLock(t)
	c := &child{ctl: t.TempDir()}
	c.cmd = exec.Command(os.Args[0], "-test.run=^TestOpLockHelperProcess$")
	c.cmd.Env = append(os.Environ(), envHelper+"="+mode, envHelpDir+"="+root, envHelpCtl+"="+c.ctl)
	if err := c.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.release(); _ = c.cmd.Wait() })
	if strings.HasPrefix(mode, "hold") {
		for i := 0; ; i++ {
			if _, err := os.Stat(filepath.Join(c.ctl, "acquired")); err == nil {
				break
			}
			if i > 1000 {
				t.Fatal("helper never acquired the lock")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	return c
}

func (c *child) release() { _ = os.WriteFile(filepath.Join(c.ctl, "release"), []byte("1"), 0o600) }

// exitCode waits for the child and returns its exit code.
func (c *child) exitCode() int {
	err := c.cmd.Wait()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

// fastWaits shortens every wait knob for the test and restores them after.
func fastWaits(t *testing.T) {
	t.Helper()
	ow, of, oe, op := lockWait, opProgressFirst, opProgressEvery, opPollSlice
	t.Cleanup(func() { lockWait, opProgressFirst, opProgressEvery, opPollSlice = ow, of, oe, op })
	lockWait, opProgressFirst, opProgressEvery, opPollSlice = 200*time.Millisecond, 50*time.Millisecond, 100*time.Millisecond, 20*time.Millisecond
}

// TestOpLockWaiterBlocksWithProgress: a writer behind a long operation in another
// process waits (past the ledger lock wait), shows progress, then succeeds.
func TestOpLockWaiterBlocksWithProgress(t *testing.T) {
	fastWaits(t)
	root := t.TempDir()
	s := storeAt(root)
	var mu sync.Mutex
	var msgs []string
	s.Progress = func(m string) { mu.Lock(); msgs = append(msgs, m); mu.Unlock() }

	c := startChild(t, "hold-op", root)
	start := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- s.Update(addPlugin("late"))
	}()
	time.Sleep(600 * time.Millisecond) // three times lockWait
	select {
	case err := <-done:
		t.Fatalf("Update returned while the operation lock was held: %v", err)
	default:
	}
	c.release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Update behind a long operation failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Update never finished after release")
	}
	if time.Since(start) < 600*time.Millisecond {
		t.Error("Update returned before the holder released the lock")
	}
	l, _, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := l.Plugins["late"]; !ok {
		t.Error("the ledger write did not happen")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(msgs) == 0 {
		t.Fatal("no progress message was shown")
	}
	pid := fmt.Sprintf("pid=%d", c.cmd.Process.Pid)
	if !strings.Contains(msgs[0], pid) || !strings.Contains(msgs[0], "op=child hold") || !strings.Contains(msgs[0], "Ctrl-C") {
		t.Errorf("progress message lacks the holder note: %q (want %s)", msgs[0], pid)
	}
}

// TestOpLockPositiveControl: a holder of the raw LEDGER lock (no operation lock)
// still makes Update fail after lockWait, so the race harness can see the old failure.
func TestOpLockPositiveControl(t *testing.T) {
	fastWaits(t)
	root := t.TempDir()
	s := storeAt(root)
	c := startChild(t, "hold-ledger", root)
	err := s.Update(addPlugin("x"))
	c.release()
	if err == nil || !strings.Contains(err.Error(), "holds the bundle ledger lock") {
		t.Fatalf("Update = %v; want the ledger lock failure", err)
	}
	if !errors.Is(err, errLockBusy) {
		t.Errorf("error does not wrap errLockBusy: %v", err)
	}
}

// TestOpLockContextCancel: a cancelled wait returns the context error and fn never runs.
func TestOpLockContextCancel(t *testing.T) {
	fastWaits(t)
	root := t.TempDir()
	s := storeAt(root)
	s.Progress = func(string) {}
	c := startChild(t, "hold-op", root)
	defer c.release()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	ran := false
	err := s.Operation(ctx, "waiter", func() error { ran = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v; want one wrapping DeadlineExceeded", err)
	}
	if ran {
		t.Error("fn ran without the lock")
	}
}

// TestOpLockReentrantWithinProcess: Update and a second Store on the same dir join
// the hold; another process sees it busy until the outer Operation returns.
func TestOpLockReentrantWithinProcess(t *testing.T) {
	needsFileLock(t)
	root := t.TempDir()
	s := storeAt(root)
	guard := time.AfterFunc(20*time.Second, func() { panic("reentrant Operation deadlocked") })
	defer guard.Stop()

	err := s.Operation(context.Background(), "outer", func() error {
		if err := s.Update(addPlugin("inner")); err != nil {
			return fmt.Errorf("Update inside Operation: %w", err)
		}
		other := storeAt(root)
		if err := other.Operation(context.Background(), "second store", func() error { return nil }); err != nil {
			return fmt.Errorf("second Operation: %w", err)
		}
		// The inner Operation returning must not have released the OS lock.
		if code := startChild(t, "probe", root).exitCode(); code != 3 {
			return fmt.Errorf("probe exit %d while held; want 3 (busy)", code)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if code := startChild(t, "probe", root).exitCode(); code != 0 {
		t.Fatalf("probe exit %d after release; want 0 (acquired)", code)
	}
}

// TestOpLockTableOfLabels: the holder note keeps printable text only.
func TestOpLockTableOfLabels(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"bundle install nchat", "bundle install nchat"},
		{"a\nb\x1b[31m", "ab[31m"},
		{"  padded  ", "padded"},
		{"", ""},
	} {
		if got := cleanNote(tc.in); got != tc.want {
			t.Errorf("cleanNote(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestOpLockSymlinkedDirIsReentrant: a second Store spelled through a symlink to
// the same plugin directory joins the hold instead of waiting on a second flock.
func TestOpLockSymlinkedDirIsReentrant(t *testing.T) {
	needsFileLock(t)
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "plugins-link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	guard := time.AfterFunc(20*time.Second, func() { panic("symlinked Operation deadlocked") })
	defer guard.Stop()
	err := storeAt(root).Operation(context.Background(), "outer", func() error {
		return storeAt(link).Operation(context.Background(), "via symlink", func() error { return nil })
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A fresh state directory under a symlinked parent must have the same key
// before and after the outer Operation creates it. This reproduces macOS
// /tmp -> /private/tmp on any Unix host, including Linux.
func TestOpLockFreshSymlinkedDirIsReentrant(t *testing.T) {
	needsFileLock(t)
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "plugins-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("create test symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, StateDirName)); !os.IsNotExist(err) {
		t.Fatalf("state directory should not exist: %v", err)
	}
	guard := time.AfterFunc(5*time.Second, func() { panic("fresh symlinked Operation deadlocked") })
	defer guard.Stop()
	err := storeAt(link).Operation(context.Background(), "outer", func() error {
		// Update takes Operation with a background context, just like the nested
		// bootstrap writer in bundle install; the guard bounds that wait too.
		if err := storeAt(link).Update(addPlugin("inner")); err != nil {
			return err
		}
		return storeAt(root).Operation(context.Background(), "real path", func() error { return nil })
	})
	if err != nil {
		t.Fatal(err)
	}
	l, _, err := storeAt(root).Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := l.Plugins["inner"]; !ok {
		t.Fatal("nested Update did not persist the plugin")
	}
	if code := startChild(t, "probe", root).exitCode(); code != 0 {
		t.Fatalf("probe exit %d after release; want 0 (acquired)", code)
	}
}
