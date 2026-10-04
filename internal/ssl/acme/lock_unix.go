//go:build !windows

package acme

// lock_unix.go: one mutating --acme run at a time per served ssl dir (two runs
// would pick the same generation number and delete each other's files): an
// exclusive flock on <ssl>/.acme/.lock, dropped by the kernel if the run dies.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Lock takes the exclusive run lock, waiting up to wait, and returns its release
// func. A busy lock returns an *Error naming the holder's pid.
func Lock(sslDir string, wait time.Duration) (release func(), err error) {
	if err = EnsureState(sslDir); err != nil {
		return nil, err
	}
	p := filepath.Join(StateDir(sslDir), ".lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for deadline := time.Now().Add(wait); syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			holder, _ := os.ReadFile(p)
			_ = f.Close()
			return nil, Refuse("wait for it to finish, then retry", "another `trust ssl ... --acme` run holds %s (pid %s)", p, strings.TrimSpace(string(holder)))
		}
	}
	_ = f.Truncate(0)
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
