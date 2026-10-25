package entitlement

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nself-org/cli/internal/license"
)

// rejectedFile sits beside the licence cache so a test or deployment that
// relocates the cache relocates the rejections with it.
const (
	rejectedFile     = "entitlement-rejected.json"
	rejectedLockFile = "entitlement-rejected.lock"
)

// rejectedMu serialises this process; the flock on rejectedLockFile serialises the rest.
var rejectedMu sync.Mutex

// Purpose: locate the persisted server-rejection record.
// Inputs: none. Outputs: path/error. Constraints: same directory as the licence cache.
// SPORT: durable per-key rejection.
func rejectedPath() (string, error) {
	cache, err := license.CachePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cache), rejectedFile), nil
}

// Purpose: load subjects the licence server refused in an earlier run.
// Inputs: record path. Outputs: key-hash set, or an error when the record exists but
// cannot be read or parsed. Constraints: only an absent file is an empty set; an
// unreadable or corrupt record is unknown state and the caller must fail closed.
// SPORT: offline refresh must not republish a server-rejected key.
func readRejected(path string) (map[string]bool, error) {
	set := make(map[string]bool)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return set, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read rejection record: %w", err)
	}
	var hashes []string
	if err := json.Unmarshal(data, &hashes); err != nil {
		return nil, fmt.Errorf("rejection record is corrupt: %w", err)
	}
	for _, h := range hashes {
		set[h] = true
	}
	return set, nil
}

// Purpose: load the persisted rejections for key selection.
// Inputs: none. Outputs: key-hash set or error. Constraints: see readRejected.
// SPORT: offline refresh must not republish a server-rejected key.
func loadRejected() (map[string]bool, error) {
	path, err := rejectedPath()
	if err != nil {
		return nil, err
	}
	return readRejected(path)
}

// Purpose: persist a server verdict for one configured key.
// Inputs: key hash and whether the server refused it. Outputs: persistence error.
// Constraints: the whole read-modify-write runs under a process mutex and a file
// lock, so concurrent refreshes never drop each other's rejections. Only an explicit
// refusal adds, only a fresh online acceptance clears. A corrupt record is replaced
// by this verdict (an online refresh re-records every configured key). A failed
// rejection write is returned: the caller must not treat the key as remembered.
// SPORT: durable per-key rejection.
func recordVerdict(hash string, rejected bool) error {
	path, err := rejectedPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	rejectedMu.Lock()
	defer rejectedMu.Unlock()
	lock, err := os.OpenFile(filepath.Join(filepath.Dir(path), rejectedLockFile), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := lockRejected(lock); err != nil {
		return err
	}
	set, readErr := readRejected(path)
	if readErr != nil {
		set = make(map[string]bool)
	} else if set[hash] == rejected {
		return nil
	}
	if rejected {
		set[hash] = true
	} else {
		delete(set, hash)
	}
	hashes := make([]string, 0, len(set))
	for h := range set {
		hashes = append(hashes, h)
	}
	data, err := json.Marshal(hashes)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".entitlement-rejected-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
