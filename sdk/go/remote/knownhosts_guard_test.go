package remote

// Purpose: tests for the pinned known_hosts guards: cross-process flock,
// foreign-line refusal and directory / file mode checks. No network.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("flock and mode bits are unix only")
	}
}

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

func TestGuard_LookupMissingDirCreatesNothing(t *testing.T) {
	p, home := pinFile(t)
	if got, err := p.Lookup("n1"); err != nil || got != nil {
		t.Fatalf("Lookup = %q, %v", got, err)
	}
	if err := p.Remove("n1"); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(home)
	if len(ents) != 0 {
		t.Errorf("Lookup/Remove created %v", ents)
	}
}

func TestForeign_RefusedEverywhere(t *testing.T) {
	const blob = "AAAAC3NzaC1lZDI1NTE5AAAAIFOREIGNFOREIGNFOREIGNFOREIGNFOREIGNFOREIGNFORE"
	cases := []string{
		"@cert-authority * ssh-ed25519 " + blob,
		"@revoked n1 ssh-ed25519 " + blob,
		"* ssh-ed25519 " + blob,
		"n1,n2 ssh-ed25519 " + blob,
		"[n1]:2222 ssh-ed25519 " + blob,
		"|1|abc=|def= ssh-ed25519 " + blob,
		"!bad ssh-ed25519 " + blob,
		"n? ssh-ed25519 " + blob,
	}
	for _, foreign := range cases {
		name := strings.Fields(foreign)[0]
		t.Run(name, func(t *testing.T) {
			p, _ := pinFile(t)
			if err := p.Add("ok", keyA); err != nil {
				t.Fatal(err)
			}
			want := "ok " + keyA + "\n# a comment\n\n" + foreign + "\n"
			if err := os.WriteFile(p.Path, []byte(want), 0o600); err != nil {
				t.Fatal(err)
			}
			ops := map[string]func() error{
				"Lookup":  func() error { _, err := p.Lookup("ok"); return err },
				"Add":     func() error { return p.Add("new", keyB) },
				"Replace": func() error { return p.Replace("ok", keyB) },
				"Remove":  func() error { return p.Remove("ok") },
			}
			for op, fn := range ops {
				err := fn()
				switch {
				case err == nil:
					t.Errorf("%s accepted a foreign line", op)
				case !strings.Contains(err.Error(), "line 4"):
					t.Errorf("%s error lacks the line number: %v", op, err)
				case !strings.Contains(err.Error(), p.Path):
					t.Errorf("%s error lacks the path: %v", op, err)
				case strings.Contains(err.Error(), blob):
					t.Errorf("%s error echoes the key blob: %v", op, err)
				}
			}
			if got, _ := os.ReadFile(p.Path); string(got) != want {
				t.Errorf("file changed: %q", got)
			}
		})
	}
}

func TestForeign_CommentAndBlankAccepted(t *testing.T) {
	p, _ := pinFile(t)
	_ = p.Add("n1", keyA)
	raw, _ := os.ReadFile(p.Path)
	_ = os.WriteFile(p.Path, append([]byte("# *,[x] @not-a-marker\n\n  # indented\n"), raw...), 0o600)
	if got, err := p.Lookup("n1"); err != nil || len(got) != 1 {
		t.Fatalf("Lookup = %q, %v", got, err)
	}
	if err := p.Add("n2", keyB); err != nil {
		t.Fatal(err)
	}
}

func TestGuard_ParentDirMode(t *testing.T) {
	skipWindows(t)
	for _, tc := range []struct {
		mode os.FileMode
		ok   bool
	}{{0o777, false}, {0o720, false}, {0o702, false}, {0o755, true}, {0o700, true}} {
		t.Run(fmt.Sprintf("%04o", tc.mode), func(t *testing.T) {
			p, _ := pinFile(t)
			if err := p.Add("n1", keyA); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Dir(p.Path)
			if err := os.Chmod(dir, tc.mode); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(p.Path)
			_, lerr := p.Lookup("n1")
			aerr := p.Add("n2", keyB)
			if tc.ok {
				if lerr != nil || aerr != nil {
					t.Fatalf("mode %04o refused: %v, %v", tc.mode, lerr, aerr)
				}
				return
			}
			for op, err := range map[string]error{"Lookup": lerr, "Add": aerr} {
				if err == nil || !strings.Contains(err.Error(), dir) || !strings.Contains(err.Error(), "chmod") {
					t.Errorf("%s with dir %04o: %v", op, tc.mode, err)
				}
			}
			if rerr := p.Remove("n1"); rerr == nil {
				t.Error("Remove accepted a writable dir")
			}
			after, _ := os.ReadFile(p.Path)
			if string(before) != string(after) {
				t.Error("pin file changed")
			}
			if fi, _ := os.Stat(dir); fi.Mode().Perm() != tc.mode {
				t.Errorf("dir was chmod-ed to %v", fi.Mode().Perm())
			}
		})
	}
}

func TestGuard_PinFileModeAndType(t *testing.T) {
	skipWindows(t)
	p, _ := pinFile(t)
	_ = p.Add("n1", keyA)
	before, _ := os.ReadFile(p.Path)
	for _, mode := range []os.FileMode{0o666, 0o660, 0o602} {
		_ = os.Chmod(p.Path, mode)
		if _, err := p.Lookup("n1"); err == nil {
			t.Errorf("Lookup accepted file mode %04o", mode)
		}
		if err := p.Add("n2", keyB); err == nil {
			t.Errorf("Add accepted file mode %04o", mode)
		}
	}
	_ = os.Chmod(p.Path, 0o644) // group/world readable is fine
	if _, err := p.Lookup("n1"); err != nil {
		t.Errorf("0644 refused: %v", err)
	}
	if after, _ := os.ReadFile(p.Path); string(after) != string(before) {
		t.Error("pin file changed")
	}

	q, _ := pinFile(t)
	if err := os.MkdirAll(q.Path, 0o700); err != nil { // the pin path is a directory
		t.Fatal(err)
	}
	if _, err := q.Lookup("n1"); err == nil {
		t.Error("Lookup accepted a directory as pin file")
	}
	if err := q.Add("n1", keyA); err == nil {
		t.Error("Add accepted a directory as pin file")
	}
}

func TestGuard_SymlinkedParentDir(t *testing.T) {
	skipWindows(t)
	for _, tc := range []struct {
		mode os.FileMode
		ok   bool
	}{{0o700, true}, {0o777, false}} {
		t.Run(fmt.Sprintf("%04o", tc.mode), func(t *testing.T) {
			p, home := pinFile(t)
			target := filepath.Join(home, "real")
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(target, tc.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Dir(p.Path)); err != nil {
				t.Fatal(err)
			}
			aerr := p.Add("n1", keyA)
			got, lerr := p.Lookup("n1")
			if tc.ok {
				if aerr != nil || lerr != nil || len(got) != 1 {
					t.Fatalf("symlinked 0700 parent refused: %v, %v, %q", aerr, lerr, got)
				}
				return
			}
			if aerr == nil || lerr == nil {
				t.Errorf("symlink to %04o dir accepted: %v, %v", tc.mode, aerr, lerr)
			}
		})
	}
}

func TestGuard_SymlinkedPinFileRefused(t *testing.T) {
	skipWindows(t)
	p, home := pinFile(t)
	_ = p.Add("n1", keyA)
	real := filepath.Join(home, "real_known_hosts")
	_ = os.Rename(p.Path, real)
	if err := os.Symlink(real, p.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Lookup("n1"); err == nil {
		t.Error("Lookup accepted a symlinked pin file")
	}
	if err := p.Add("n2", keyB); err == nil {
		t.Error("Add accepted a symlinked pin file")
	}
}
