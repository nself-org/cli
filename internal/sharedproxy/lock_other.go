//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package sharedproxy

// lock_other.go — the lock on platforms without flock (Windows).
//
// There the lock only serialises the calls of one process (a mutex per owner
// dir). Two nself processes are not excluded; Windows is supported through
// WSL2 (VMI), where the unix build runs. internal/oplock makes the same trade.

import (
	"os"
	"sync"
)

var (
	procLocksMu sync.Mutex
	procLocks   = map[string]*sync.Mutex{}
)

// tryLock takes the in-process mutex of key. f is unused here.
func tryLock(_ *os.File, key string) (unlock func(), ok bool, err error) {
	procLocksMu.Lock()
	m := procLocks[key]
	if m == nil {
		m = &sync.Mutex{}
		procLocks[key] = m
	}
	procLocksMu.Unlock()
	if !m.TryLock() {
		return nil, false, nil
	}
	return m.Unlock, true, nil
}
