package invoke

// Purpose: the Guard (one per server process) and the request-confirmation
// nonce store behind the gate of contract:cli.machine-request v1 (EPIC D3).
// Inputs: a request digest to bind a nonce to; a nonce plus the digest of the
// repeated request to spend it.
// Outputs: server-issued nonces (32 random bytes, hex) and a yes/no on spend.
// Constraints: a nonce is single use, lives 10 minutes and dies with the
// process; it is bound to the sha256 of the canonical JSON of the full
// unredacted request (command, args, argv, flags; confirm excluded). The store
// is keyed by the sha256 of the nonce, holds at most maxLiveNonces entries
// (expired first, then oldest out) and a nonce is never logged, echoed in an
// error or written to the audit file. Spending takes the mutex, so two
// concurrent calls with one nonce run exactly one.

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// nonceTTL is how long an issued nonce stays valid (EPIC D3).
const nonceTTL = 10 * time.Minute

// maxLiveNonces bounds the store so a client cannot grow memory without limit.
// A variable so a test can lower it.
var maxLiveNonces = 1024

// Guard is the gate state of one server process: the nonce store, the audit
// path and the clock. Build it once with NewGuard and pass it in Options.
type Guard struct {
	// Now is the clock; tests inject a fixed or advancing one. Set it before
	// the Guard is shared between goroutines.
	Now func() time.Time

	auditPath string

	mu     sync.Mutex
	seq    uint64
	nonces map[[32]byte]nonceEntry
}

type nonceEntry struct {
	digest [32]byte
	expiry time.Time
	seq    uint64
}

// NewGuard returns a Guard that audits to auditPath (DefaultAuditPath is the
// production value). An empty path disables the audit file.
func NewGuard(auditPath string) *Guard {
	return &Guard{Now: time.Now, auditPath: auditPath, nonces: map[[32]byte]nonceEntry{}}
}

// now is the Guard clock; a nil Guard uses the wall clock.
func (g *Guard) now() time.Time {
	if g == nil || g.Now == nil {
		return time.Now()
	}
	return g.Now()
}

// issue stores a fresh nonce bound to digest and returns it.
func (g *Guard) issue(digest [32]byte) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(raw)
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.nonces == nil {
		g.nonces = map[[32]byte]nonceEntry{}
	}
	if len(g.nonces) >= maxLiveNonces {
		g.evictLocked(now)
	}
	g.seq++
	g.nonces[sha256.Sum256([]byte(nonce))] = nonceEntry{digest: digest, expiry: now.Add(nonceTTL), seq: g.seq}
	return nonce, nil
}

// evictLocked makes room: expired entries first, then the oldest ones.
func (g *Guard) evictLocked(now time.Time) {
	for k, e := range g.nonces {
		if !now.Before(e.expiry) {
			delete(g.nonces, k)
		}
	}
	for len(g.nonces) >= maxLiveNonces {
		var oldest [32]byte
		var low uint64
		first := true
		for k, e := range g.nonces {
			if first || e.seq < low {
				oldest, low, first = k, e.seq, false
			}
		}
		delete(g.nonces, oldest)
	}
}

// consume spends nonce for the request with digest. It is true only for a live,
// unused nonce issued for exactly this request. A known nonce is spent whether
// or not the digest matches: presenting it for another request burns it.
func (g *Guard) consume(nonce string, digest [32]byte) bool {
	key := sha256.Sum256([]byte(nonce))
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.nonces[key]
	if !ok {
		return false
	}
	delete(g.nonces, key)
	if !now.Before(e.expiry) {
		return false
	}
	return subtle.ConstantTimeCompare(e.digest[:], digest[:]) == 1
}

// liveNonces is the store size, for tests.
func (g *Guard) liveNonces() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.nonces)
}

// requestDigest is the sha256 of the canonical JSON of the full unredacted
// request: {"args","argv","command","flags"}, keys sorted, no whitespace,
// confirm excluded. It is never logged. ok is false when a flag value cannot
// be encoded.
func requestDigest(command string, r Request) (sum [32]byte, ok bool) {
	doc := map[string]any{"args": []string{}, "argv": []string{}, "command": command, "flags": map[string]any{}}
	if r.Args != nil {
		doc["args"] = r.Args
	}
	if r.Argv != nil {
		doc["argv"] = r.Argv
	}
	if r.Flags != nil {
		flags := make(map[string]any, len(r.Flags))
		for name, v := range r.Flags {
			flags[name] = canonValue(v)
		}
		doc["flags"] = flags
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return sum, false
	}
	return sha256.Sum256(bytes.TrimRight(buf.Bytes(), "\n")), true
}

// auditWritable checks, before a remote or destructive run, that the audit
// file can be opened for append. The audit line is written after the run, so
// without this probe a broken log (no path, unwritable dir) would lose the
// record of a destructive change; the run is refused instead. A failure after
// the probe (disk full) still only prints a notice.
func (g *Guard) auditWritable() error {
	if g.auditPath == "" {
		return errors.New("this server has no machine audit log")
	}
	auditFileMu.Lock()
	defer auditFileMu.Unlock()
	if err := ensureAuditDir(filepath.Dir(g.auditPath)); err != nil {
		return err
	}
	f, err := openAuditFile(g.auditPath)
	if err != nil {
		return err
	}
	return f.Close()
}

// openAuditFile opens path for append (0600, created when missing) and
// refuses anything but a regular file: a symlink planted at the audit path
// would redirect the log and let the 0600 narrowing chmod its target.
func openAuditFile(path string) (*os.File, error) {
	if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
		return nil, errors.New("the machine audit path is not a regular file")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	lst, lerr := os.Lstat(path)
	fst, ferr := f.Stat()
	if lerr != nil || ferr != nil || !lst.Mode().IsRegular() || !os.SameFile(lst, fst) {
		_ = f.Close()
		return nil, errors.New("the machine audit path changed while it was opened")
	}
	return f, nil
}
