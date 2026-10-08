package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	// probeConcurrencyLimit is the global maximum number of simultaneous SSH
	// probe goroutines. It prevents thundering-herd against large inventories.
	probeConcurrencyLimit = 8

	// probeCacheTTL is how long a cached capability result is considered valid.
	probeCacheTTL = 60 * time.Second

	// probeCacheFile is the file (relative to .nself/) where probe results are
	// cached between invocations.
	probeCacheFile = "state/capability.json"

	// probeConnectTimeout is the SSH connect timeout in seconds. This value is
	// passed to ssh's -o ConnectTimeout option.
	probeConnectTimeout = 5
)

// cachedEntry is one entry in the on-disk probe cache.
type cachedEntry struct {
	Host     string    `json:"host"`
	ProbeAt  time.Time `json:"probed_at"`
	SSH      bool      `json:"ssh_reachable"`
	Docker   bool      `json:"docker_ok"`
	LatencyM int       `json:"latency_ms"`
}

// SSHProber is the production implementation of Prober. It performs real SSH
// and docker probes, enforces a global concurrency limit, maintains a per-host
// mutex to prevent duplicate concurrent probes to the same host, and caches
// results to .nself/state/capability.json (0600) for probeCacheTTL.
type SSHProber struct {
	// projectRoot is the working directory used to locate .nself/state/.
	projectRoot string

	// refresh bypasses the on-disk cache when true (--refresh flag).
	refresh bool

	// sem is a buffered channel used as a counting semaphore; capacity =
	// probeConcurrencyLimit.
	sem chan struct{}

	// mu serialises access to the per-host mutex map and the cache map.
	mu sync.Mutex

	// hostMu holds one mutex per target host so that two goroutines probing
	// the same host block on each other rather than issuing duplicate SSH calls.
	hostMu map[string]*sync.Mutex

	// memCache holds in-process results for the lifetime of this prober.
	memCache map[string]cachedEntry
}

// NewSSHProber constructs a production Prober. projectRoot is the working
// directory (parent of .nself/). When refresh is true the on-disk cache is
// ignored for this run.
func NewSSHProber(projectRoot string, refresh bool) *SSHProber {
	return &SSHProber{
		projectRoot: projectRoot,
		refresh:     refresh,
		sem:         make(chan struct{}, probeConcurrencyLimit),
		hostMu:      make(map[string]*sync.Mutex),
		memCache:    make(map[string]cachedEntry),
	}
}

// KeyState checks whether the SSH key referenced by s.SSHKeyRef exists on the
// local filesystem. It does not perform any network I/O.
func (sp *SSHProber) KeyState(s Server) (present bool, path string) {
	if s.SSHKeyRef == "" {
		return false, ""
	}
	keyPath := os.Getenv(s.SSHKeyRef)
	if keyPath == "" {
		return false, ""
	}
	_, err := os.Stat(keyPath)
	return err == nil, keyPath
}

// SSHReachable performs an SSH probe to s.Host using the key from s.SSHKeyRef.
// The exact argument vector is:
//
//	ssh -i <key> -o BatchMode=yes -o StrictHostKeyChecking=accept-new
//	    -o ConnectTimeout=5 -o ForwardAgent=no <user@host> "true"
//
// Results are cached to .nself/state/capability.json for probeCacheTTL.
// The global semaphore limits concurrent probes to probeConcurrencyLimit.
// A per-host mutex prevents duplicate probes to the same host.
func (sp *SSHProber) SSHReachable(s Server) (bool, int, error) {
	hostMu := sp.hostMutex(s.Host)
	hostMu.Lock()
	defer hostMu.Unlock()

	// Check in-process cache first.
	if !sp.refresh {
		if entry, ok := sp.cachedEntry(s.Host); ok {
			return entry.SSH, entry.LatencyM, nil
		}
	}

	// Acquire global concurrency slot.
	sp.sem <- struct{}{}
	defer func() { <-sp.sem }()

	keyPath := os.Getenv(s.SSHKeyRef)
	if keyPath == "" {
		return false, 0, fmt.Errorf("env var %s is not set", s.SSHKeyRef)
	}

	spec, err := remote.ParseHostSpec(s.Host)
	if err != nil {
		return false, 0, err
	}
	tier, envName := probeTier(sp.projectRoot, s)
	keyCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	policy, err := HostKeyOptions(keyCtx, envName, s.Name, tier, s.Host, false)
	cancel()
	if err != nil {
		return false, 0, err
	}
	args := []string{
		"-i", keyPath,
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", probeConnectTimeout),
		"-o", "ForwardAgent=no",
	}
	args = append(args, policy...)
	args = append(args, spec.SSHArgs()...)
	args = append(args, "true")

	start := time.Now()
	cmd := exec.Command("ssh", args...) //nolint:gosec // args are fully controlled
	err = cmd.Run()
	latencyMS := int(time.Since(start).Milliseconds())

	reachable := err == nil
	entry := cachedEntry{
		Host:     s.Host,
		ProbeAt:  time.Now(),
		SSH:      reachable,
		LatencyM: latencyMS,
	}

	sp.storeEntry(s.Host, entry)
	_ = sp.persistCache()

	return reachable, latencyMS, err
}

// DockerOK runs "docker info" over SSH to verify the Docker daemon is running
// on s. It is only called by the resolver when SSHReachable returned true.
func (sp *SSHProber) DockerOK(s Server) (bool, error) {
	keyPath := os.Getenv(s.SSHKeyRef)
	if keyPath == "" {
		return false, fmt.Errorf("env var %s is not set", s.SSHKeyRef)
	}

	spec, err := remote.ParseHostSpec(s.Host)
	if err != nil {
		return false, err
	}
	tier, envName := probeTier(sp.projectRoot, s)
	keyCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	policy, err := HostKeyOptions(keyCtx, envName, s.Name, tier, s.Host, false)
	cancel()
	if err != nil {
		return false, err
	}
	args := []string{
		"-i", keyPath,
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", probeConnectTimeout),
		"-o", "ForwardAgent=no",
	}
	args = append(args, policy...)
	args = append(args, spec.SSHArgs()...)
	args = append(args, "docker info --format '{{.ID}}' 2>/dev/null")

	cmd := exec.Command("ssh", args...) //nolint:gosec
	err = cmd.Run()
	dockerOK := err == nil

	// Update the cache entry with docker result.
	sp.mu.Lock()
	if entry, ok := sp.memCache[s.Host]; ok {
		entry.Docker = dockerOK
		sp.memCache[s.Host] = entry
	}
	sp.mu.Unlock()
	_ = sp.persistCache()

	return dockerOK, nil
}

// hostMutex returns the per-host mutex for h, creating it if needed.
func (sp *SSHProber) hostMutex(h string) *sync.Mutex {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if _, ok := sp.hostMu[h]; !ok {
		sp.hostMu[h] = &sync.Mutex{}
	}
	return sp.hostMu[h]
}

// cachedEntry returns the in-process cached entry for host h if it is still
// within probeCacheTTL. It also reads the on-disk cache on first access.
func (sp *SSHProber) cachedEntry(h string) (cachedEntry, bool) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if len(sp.memCache) == 0 {
		// Attempt to populate from disk cache.
		_ = sp.loadDiskCache()
	}

	entry, ok := sp.memCache[h]
	if !ok {
		return cachedEntry{}, false
	}
	if time.Since(entry.ProbeAt) > probeCacheTTL {
		delete(sp.memCache, h)
		return cachedEntry{}, false
	}
	return entry, true
}

// storeEntry stores entry in the in-process cache. Caller must not hold mu.
func (sp *SSHProber) storeEntry(h string, entry cachedEntry) {
	sp.mu.Lock()
	sp.memCache[h] = entry
	sp.mu.Unlock()
}

// loadDiskCache reads the on-disk cache file. Must be called with sp.mu held.
func (sp *SSHProber) loadDiskCache() error {
	path := filepath.Join(sp.projectRoot, ".nself", probeCacheFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err // not fatal; cache is optional
	}
	var entries []cachedEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	for _, e := range entries {
		if time.Since(e.ProbeAt) <= probeCacheTTL {
			sp.memCache[e.Host] = e
		}
	}
	return nil
}

// persistCache writes the in-process cache to .nself/state/capability.json.
// The file is created with mode 0600. Errors are non-fatal.
func (sp *SSHProber) persistCache() error {
	sp.mu.Lock()
	entries := make([]cachedEntry, 0, len(sp.memCache))
	for _, e := range sp.memCache {
		entries = append(entries, e)
	}
	sp.mu.Unlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Join(sp.projectRoot, ".nself", "state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "capability.json")
	return os.WriteFile(path, data, 0o600)
}
