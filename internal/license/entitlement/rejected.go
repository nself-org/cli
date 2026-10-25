package entitlement

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/license"
)

// rejectedFile sits beside the licence cache so a test or deployment that
// relocates the cache relocates the rejections with it.
const rejectedFile = "entitlement-rejected.json"

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
// Inputs: none. Outputs: key-hash set. Constraints: a missing or unreadable file
// is empty (the next online refresh re-establishes it); never errors a refresh.
// SPORT: offline refresh must not republish a server-rejected key.
func loadRejected() map[string]bool {
	set := make(map[string]bool)
	path, err := rejectedPath()
	if err != nil {
		return set
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return set
	}
	var hashes []string
	if json.Unmarshal(data, &hashes) != nil {
		return set
	}
	for _, h := range hashes {
		set[h] = true
	}
	return set
}

// Purpose: persist a server verdict for one configured key.
// Inputs: key hash and whether the server refused it. Outputs: none.
// Constraints: only an explicit refusal adds, only a fresh online acceptance
// clears; write failures are non-fatal because the in-run map still applies.
// SPORT: durable per-key rejection.
func recordVerdict(hash string, rejected bool) {
	set := loadRejected()
	if set[hash] == rejected {
		return
	}
	if rejected {
		set[hash] = true
	} else {
		delete(set, hash)
	}
	path, err := rejectedPath()
	if err != nil {
		return
	}
	hashes := make([]string, 0, len(set))
	for h := range set {
		hashes = append(hashes, h)
	}
	data, err := json.Marshal(hashes)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".entitlement-rejected-*")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return
	}
	if tmp.Chmod(0600) != nil || tmp.Close() != nil {
		return
	}
	_ = os.Rename(tmp.Name(), path)
}
