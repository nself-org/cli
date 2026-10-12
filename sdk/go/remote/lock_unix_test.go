//go:build unix && !aix && !solaris && !illumos

package remote

// Purpose: unix-only tests for the pin store's owner rule and the O_NOFOLLOW
// lock open. No network, no root needed.

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestOwner_UIDRule(t *testing.T) {
	for _, tc := range []struct {
		uid, euid int
		ok        bool
	}{{501, 501, true}, {0, 501, true}, {0, 0, true}, {502, 501, false}, {501, 0, false}, {1, 0, false}} {
		if got := uidOK(tc.uid, tc.euid); got != tc.ok {
			t.Errorf("uidOK(%d, %d) = %v, want %v", tc.uid, tc.euid, got, tc.ok)
		}
	}
}

func TestOwner_OwnFileAndDirPass(t *testing.T) {
	dir := safeTempDir(t) // 0700, and no writable ancestor under compat.V15
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(f)
	if st := fi.Sys().(*syscall.Stat_t); int(st.Uid) != os.Geteuid() {
		t.Fatalf("temp file uid %d != euid %d", st.Uid, os.Geteuid())
	}
	if !ownerOK(fi) {
		t.Error("ownerOK refused a file we own")
	}
	if err := checkFile(f); err != nil {
		t.Errorf("checkFile: %v", err)
	}
	if err := checkDir(dir); err != nil {
		t.Errorf("checkDir: %v", err)
	}
}

func TestLock_SymlinkedLockFileRefused(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		p, home := pinFile(t)
		if err := p.Add("n1", keyA); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(home, "elsewhere")
		if !dangling {
			if err := os.WriteFile(target, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		lock := p.Path + ".lock"
		if err := os.Remove(lock); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, lock); err != nil {
			t.Fatal(err)
		}
		before := readPin(t, p)
		if err := p.Add("n2", keyB); err == nil {
			t.Errorf("dangling=%v: Add followed a symlinked lock file", dangling)
		}
		if _, err := p.Lookup("n1"); err == nil {
			t.Errorf("dangling=%v: Lookup followed a symlinked lock file", dangling)
		}
		if err := p.Remove("n1"); err == nil {
			t.Errorf("dangling=%v: Remove followed a symlinked lock file", dangling)
		}
		if readPin(t, p) != before {
			t.Errorf("dangling=%v: pin file changed", dangling)
		}
		if _, err := os.Stat(target); !dangling && err != nil || dangling && err == nil {
			t.Errorf("dangling=%v: target touched (%v)", dangling, err)
		}
	}
}

func TestLock_FIFOLockFileRefusedPromptly(t *testing.T) {
	p, _ := pinFile(t)
	if err := p.Add("n1", keyA); err != nil {
		t.Fatal(err)
	}
	lock := p.Path + ".lock"
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(lock, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan [3]error, 1)
	go func() {
		_, lerr := p.Lookup("n1")
		done <- [3]error{lerr, p.Add("n2", keyB), p.Remove("n1")}
	}()
	select {
	case errs := <-done:
		for i, err := range errs {
			if err == nil || !strings.Contains(err.Error(), "not a regular file") {
				t.Errorf("op %d on a FIFO lock: %v", i, err)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FIFO at the lock path hung the call")
	}
}

func TestLock_GroupWritableLockFileRefused(t *testing.T) {
	p, _ := pinFile(t)
	if err := p.Add("n1", keyA); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p.Path+".lock", 0o660); err != nil {
		t.Fatal(err)
	}
	if err := p.Add("n2", keyB); err == nil {
		t.Error("Add accepted a group-writable lock file")
	}
	if _, err := p.Lookup("n1"); err == nil {
		t.Error("Lookup accepted a group-writable lock file")
	}
}
