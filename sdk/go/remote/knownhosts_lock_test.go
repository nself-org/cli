package remote

// Purpose: tests for the pinned known_hosts cross-process lock (flock on the
// sidecar). No network.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestHelperPinWriter is the child half of TestLock_CrossProcess; it is a
// no-op in a normal run.
func TestHelperPinWriter(t *testing.T) {
	path := os.Getenv("PIN_HELPER_PATH")
	if path == "" {
		return
	}
	for i := 0; i < 500; i++ { // start barrier so the children collide
		if _, err := os.Stat(os.Getenv("PIN_HELPER_GO")); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := (PinnedHostKeys{Path: path}).Add(os.Getenv("PIN_HELPER_ALIAS"), keyA); err != nil {
		t.Fatal(err)
	}
}

func TestLock_CrossProcess(t *testing.T) {
	skipWindows(t)
	p, _ := pinFile(t)
	goFile := filepath.Join(t.TempDir(), "go")
	const n = 8
	var cmds []*exec.Cmd
	for i := 0; i < n; i++ {
		c := exec.Command(os.Args[0], "-test.run=^TestHelperPinWriter$", "-test.count=1")
		c.Env = append(os.Environ(), "PIN_HELPER_PATH="+p.Path, "PIN_HELPER_GO="+goFile,
			fmt.Sprintf("PIN_HELPER_ALIAS=proc-%d", i))
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, c)
	}
	if err := os.WriteFile(goFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for i, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Errorf("child %d: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		alias := fmt.Sprintf("proc-%d", i)
		if got, err := p.Lookup(alias); err != nil || len(got) != 1 {
			t.Errorf("%s lost: %q, %v", alias, got, err)
		}
	}
}

func TestLock_WriterBlocksOnHeldLock(t *testing.T) {
	skipWindows(t)
	p, _ := pinFile(t)
	if err := p.Add("seed", keyA); err != nil { // creates dir and lock file
		t.Fatal(err)
	}
	f, err := os.OpenFile(p.Path+".lock", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := lockFD(f, true); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Add("late", keyB) }()
	select {
	case err := <-done:
		t.Fatalf("Add finished while the lock was held: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	unlockFD(f)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Add did not finish after the lock was released")
	}
	if got, _ := p.Lookup("late"); len(got) != 1 {
		t.Errorf("late = %q", got)
	}
}

func TestLock_ReaderBlocksOnWriter(t *testing.T) {
	skipWindows(t)
	p, _ := pinFile(t)
	_ = p.Add("seed", keyA)
	f, _ := os.OpenFile(p.Path+".lock", os.O_RDWR, 0)
	defer f.Close()
	if err := lockFD(f, true); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var once sync.Once
	go func() { _, _ = p.Lookup("seed"); once.Do(func() { close(done) }) }()
	select {
	case <-done:
		t.Fatal("Lookup finished while a writer held LOCK_EX")
	case <-time.After(300 * time.Millisecond):
	}
	unlockFD(f)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Lookup did not finish after release")
	}
}

// holdLock takes the lock file of p from a second descriptor, as another
// process would, and returns the release func.
func holdLock(t *testing.T, p PinnedHostKeys, exclusive bool) func() {
	t.Helper()
	f, err := os.OpenFile(p.Path+".lock", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := lockFD(f, exclusive); err != nil {
		t.Fatal(err)
	}
	return func() { unlockFD(f); _ = f.Close() }
}

func TestLock_HungHolderTimesOut(t *testing.T) {
	skipWindows(t)
	old := pinLockWait
	pinLockWait = 100 * time.Millisecond
	t.Cleanup(func() { pinLockWait = old })
	p, _ := pinFile(t)
	if err := p.Add("seed", keyA); err != nil {
		t.Fatal(err)
	}
	before := readPin(t, p)
	release := holdLock(t, p, true)
	done := make(chan [3]error, 1)
	go func() {
		_, lerr := p.Lookup("seed")
		done <- [3]error{lerr, p.Add("late", keyB), p.Remove("seed")}
	}()
	select {
	case errs := <-done:
		for i, err := range errs {
			if err == nil || !strings.Contains(err.Error(), "held by another process") || !strings.Contains(err.Error(), p.Path+".lock") {
				t.Errorf("op %d with the lock held: %v", i, err)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a held lock blocked past pinLockWait")
	}
	if readPin(t, p) != before {
		t.Error("pin file changed while the lock was held")
	}
	release() // positive control: the same calls succeed once it is free
	if err := p.Add("late", keyB); err != nil {
		t.Fatal(err)
	}
	if got, err := p.Lookup("late"); err != nil || len(got) != 1 {
		t.Errorf("late = %q, %v", got, err)
	}
}

func TestLock_SharedHolderLetsReadersIn(t *testing.T) {
	skipWindows(t)
	old := pinLockWait
	pinLockWait = 100 * time.Millisecond
	t.Cleanup(func() { pinLockWait = old })
	p, _ := pinFile(t)
	if err := p.Add("seed", keyA); err != nil {
		t.Fatal(err)
	}
	release := holdLock(t, p, false)
	defer release()
	if got, err := p.Lookup("seed"); err != nil || len(got) != 1 {
		t.Errorf("Lookup beside a shared holder = %q, %v", got, err)
	}
	if err := p.Add("late", keyB); err == nil || !strings.Contains(err.Error(), "held by another process") {
		t.Errorf("Add beside a shared holder: %v", err)
	}
}
