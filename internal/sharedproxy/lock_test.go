package sharedproxy

// Tests for lock.go: the public entry points are serialised per owner dir.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// overlapDocker is the fake docker with a busy marker between `network
// connect` and `nginx -s reload`: a second connect while it is set logs
// overlap.log. `nginx -t` sleeps so unlocked calls would interleave.
func overlapDocker(t *testing.T) {
	t.Helper()
	script := strings.Replace(fakeProxyDocker, `connect) echo "$4" >> "$FAKE_DIR/nets.txt" ;;`,
		`connect) mkdir "$FAKE_DIR/busy" 2>/dev/null || echo overlap >> "$FAKE_DIR/overlap.log"; echo "$4" >> "$FAKE_DIR/nets.txt" ;;`, 1)
	script = strings.Replace(script, `if [ "$4" = "-t" ]; then`, `if [ "$4" = "-t" ]; then sleep 0.15`, 1)
	script = strings.Replace(script, `echo reload >> "$FAKE_DIR/reloads.log"`, `rmdir "$FAKE_DIR/busy"; echo reload >> "$FAKE_DIR/reloads.log"`, 1)
	if script == fakeProxyDocker {
		t.Fatal("the fake docker script changed: update overlapDocker")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func shortLockWait(t *testing.T, d time.Duration) {
	t.Helper()
	old := lockWait
	lockWait = d
	t.Cleanup(func() { lockWait = old })
}

func TestConcurrentAttachesNeverOverlap(t *testing.T) {
	f := newProxyFixture(t, "b_network", "c_network")
	f.owner0()
	b, c := f.project("b"), f.project("c")
	overlapDocker(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errc := make(chan error, 2)
	for _, r := range []Registration{b, c} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Attach(ctx, f.owner, r.Project, []Registration{r})
			errc <- err
		}()
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		if err != nil {
			t.Errorf("attach: %v", err)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(f.fake, "overlap.log")); len(got) != 0 {
		t.Errorf("two attaches of one owner overlapped:\n%s", got)
	}
	if f.reloads() != 2 {
		t.Errorf("reloads = %d, want 2", f.reloads())
	}
}

// Every public entry point waits for a held lock and runs after the release.
func TestEntryPointsWaitForTheLock(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	set, err := Load(DirOf(f.owner), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]func() error{
		"Attach":     func() error { _, err := Attach(ctx, f.owner, "b", []Registration{b}); return err },
		"Detach":     func() error { return Detach(ctx, f.owner, b) },
		"Leave":      func() error { _, err := Leave(ctx, f.owner, "b"); return err },
		"Unregister": func() error { _, err := Unregister(f.owner, "b"); return err },
		"Quarantine": func() error { _, err := Quarantine(f.owner, set, b); return err },
		"Restore":    func() error { _, err := Restore(f.owner, b); return err },
	}
	for name, call := range calls {
		release, err := Lock(ctx, f.owner)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- call() }()
		select {
		case <-done:
			t.Fatalf("%s ran while the owner lock was held", name)
		case <-time.After(150 * time.Millisecond):
		}
		release()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s did not run after the lock was released", name)
		}
		f.project("b") // Leave and Unregister remove it: put it back for the next call
	}
}

func TestLockTimesOutWithE506(t *testing.T) {
	f := newProxyFixture(t)
	f.owner0()
	release, err := Lock(context.Background(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	shortLockWait(t, 100*time.Millisecond)
	if _, err := Restore(f.owner, f.project("b")); !isE506(err) {
		t.Fatalf("a lock that stays held must end in E506, got %v", err)
	}
}

func TestLockFileIsPrivateAndInsideTheOwner(t *testing.T) {
	f := newProxyFixture(t)
	f.owner0()
	release, err := Lock(context.Background(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	release()
	fi, err := os.Lstat(filepath.Join(DirOf(f.owner), LockFile))
	if err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("lock file: %v %v", fi, err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("lock file mode %v, want 0600", fi.Mode().Perm())
	}
	// An owner with no nginx/shared takes no lock and creates nothing.
	bare := t.TempDir()
	release, err = Lock(context.Background(), bare)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if _, err := os.Stat(filepath.Join(bare, "nginx")); err == nil {
		t.Error("Lock must not create nginx/ in an owner without a shared proxy")
	}
}

// A network a concurrent start connected while this call waited for the lock is
// not "connected by this call": a later failure must not disconnect it.
func TestAttachSnapshotsNetworksAfterTheLock(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/other.conf\n") // names no registered file
	f.write(filepath.Join(f.owner, "nginx", "sites", "other.conf"), "bogus\n")
	release, err := Lock(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := Attach(ctx, f.owner, "", []Registration{b}); done <- err }()
	time.Sleep(100 * time.Millisecond)
	f.write(filepath.Join(f.fake, "nets.txt"), "bridge\nb_network\n") // B's own start connected it
	release()
	if err := <-done; !isE506(err) {
		t.Fatalf("want E506, got %v", err)
	}
	if !strings.Contains(f.nets(), "b_network") {
		t.Errorf("the rollback disconnected a network another start connected:\n%s", f.calls())
	}
}
