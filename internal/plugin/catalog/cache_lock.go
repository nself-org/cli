package catalog

// cache_lock.go — serialise access to the on-disk catalog pair.
//
// Purpose: the body and the ETag are two files, so two writers (or a writer and
// a reader) could interleave and pair one response's body with another's ETag;
// a later 304 would then keep the wrong body (D-0875). Every read and write of
// the pair holds one lock.
// Inputs: the cache directory. Outputs: an unlock func.
// Constraints: a process-wide mutex covers goroutines of one process; an flock
// on catalog.lock beside the files covers other processes (cache_lock_unix.go;
// a no-op without flock, where the mutex still holds). The lock is never held
// across a network call.

import (
	"os"
	"path/filepath"
	"sync"
)

const lockFile = "catalog.lock"

var cacheMu sync.Mutex

// lockCache takes the cache lock for dir. The returned func releases it. An
// error means the cross-process lock could not be taken; nothing is held then.
func lockCache(dir string) (func(), error) {
	cacheMu.Lock()
	f, err := os.OpenFile(filepath.Join(dir, lockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		cacheMu.Unlock()
		return nil, err
	}
	if err := flockFile(f); err != nil {
		_ = f.Close()
		cacheMu.Unlock()
		return nil, err
	}
	return func() {
		_ = f.Close() // closing drops the flock
		cacheMu.Unlock()
	}, nil
}
