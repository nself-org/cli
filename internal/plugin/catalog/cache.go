package catalog

// cache.go — the on-disk copy of the catalog.
//
// Purpose: keep the last good body and its ETag so the next fetch can send
// If-None-Match, and so search and the unknown-command hint can answer offline.
// Inputs: the cache directory. Outputs: catalog.json (the body, as served) and
// catalog.etag beside it, both mode 0600.
// Constraints: the ETag file is removed before the body is replaced and
// written after it, so a crash between the two leaves a body with no ETag
// (an unconditional refetch), never a new body paired with an old ETag. Both
// files are read and written under one lock (cache_lock.go), so concurrent
// processes cannot pair one response's body with another's ETag (D-0875).
// Writes are best effort: a read-only cache dir never fails a command.

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

const (
	bodyFile = "catalog.json"
	etagFile = "catalog.etag"
)

// DefaultCacheDir is where the catalog cache lives: NSELF_PLUGIN_CACHE (the
// registry cache override) or ~/.nself/cache/plugins. Empty when neither can
// be resolved; an empty dir disables caching.
func DefaultCacheDir() string {
	if dir := os.Getenv("NSELF_PLUGIN_CACHE"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".nself", "cache", "plugins")
}

// cached is the stored body, its ETag and when it was last confirmed.
type cached struct {
	body []byte
	etag string
	at   time.Time
}

// readCache returns the cached body and ETag. A missing file, an unreadable
// file or a body that no longer decodes all count as "no cache".
func readCache(dir string) (*cached, *Catalog, bool) {
	if dir == "" {
		return nil, nil, false
	}
	// Best effort: with no lock (dir missing, read-only) the read goes ahead.
	if unlock, err := lockCache(dir); err == nil {
		defer unlock()
	}
	path := filepath.Join(dir, bodyFile)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBody {
		return nil, nil, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false
	}
	cat, err := Decode(body)
	if err != nil {
		return nil, nil, false
	}
	etag, _ := os.ReadFile(filepath.Join(dir, etagFile))
	return &cached{body: body, etag: strings.TrimSpace(string(etag)), at: info.ModTime()}, cat, true
}

// writeCache stores body and etag (empty etag stores none).
func writeCache(dir string, body []byte, etag string) {
	if dir == "" || os.MkdirAll(dir, 0o755) != nil {
		return
	}
	// No lock, no write: an unserialised write is what pairs a body with the
	// wrong ETag, and the cache is optional.
	unlock, err := lockCache(dir)
	if err != nil {
		return
	}
	defer unlock()
	if err := os.Remove(filepath.Join(dir, etagFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return
	}
	if writeAtomic(filepath.Join(dir, bodyFile), body) != nil {
		return
	}
	if etag != "" {
		_ = writeAtomic(filepath.Join(dir, etagFile), []byte(etag+"\n"))
	}
}

// touchCache marks the cached body as confirmed current just now (a 304).
func touchCache(dir string, now time.Time) {
	_ = os.Chtimes(filepath.Join(dir, bodyFile), now, now)
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".catalog-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(name)
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// LoadCached returns the cached catalog without any network access. It is the
// only read the unknown-command path makes. E115 when there is no usable cache.
func LoadCached(dir string) (Result, error) {
	c, cat, ok := readCache(dir)
	if !ok {
		return Result{}, unavailable("no cached catalog yet; run nself plugin search once while online")
	}
	return Result{Catalog: cat, Source: SourceCache, FetchedAt: c.at}, nil
}

// CommandHint returns the slug of the installable catalog plugin whose
// command is cmd, from the cache alone. ok is false when there is no cache or
// no such plugin. It never touches the network and never fails the caller.
func CommandHint(cmd string) (slug string, ok bool) {
	res, err := LoadCached(DefaultCacheDir())
	if err != nil {
		return "", false
	}
	e, found := res.Catalog.FindByCommand(cmd)
	return e.Slug, found
}

// readBody reads at most maxBody bytes and reports a larger body as E116.
func readBody(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBody {
		return nil, invalid("body is larger than %d bytes", maxBody)
	}
	return b, nil
}

func unavailable(format string, args ...any) error {
	return errs.Newf("E115", "plugin catalog unavailable: "+format, args...)
}
