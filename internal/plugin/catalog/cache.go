package catalog

// cache.go — the on-disk copy of the catalog.
//
// Purpose: keep the last good body and its ETag so the next fetch can send
// If-None-Match, and so search and the unknown-command hint can answer offline.
// Inputs: the cache directory. Outputs: catalog.cache.json, mode 0600.
// Constraints: body and ETag share one atomically replaced record. A mutex
// serializes local writers; atomic rename also keeps other processes' reads
// paired. Legacy bodies remain usable offline, without trusting their ETags.
// Writes are best effort: a read-only cache dir never fails a command.

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

const (
	bodyFile   = "catalog.json"
	recordFile = "catalog.cache.json"
)

var cacheMu sync.Mutex

type cacheRecord struct {
	Body []byte `json:"body"`
	ETag string `json:"etag"`
}

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
	path := filepath.Join(dir, recordFile)
	f, err := os.Open(path)
	legacy := errors.Is(err, os.ErrNotExist)
	if legacy {
		f, err = os.Open(filepath.Join(dir, bodyFile))
	}
	if err != nil {
		return nil, nil, false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2*maxBody {
		return nil, nil, false
	}
	body, err := io.ReadAll(io.LimitReader(f, 2*maxBody+1))
	if err != nil {
		return nil, nil, false
	}
	etag := ""
	if !legacy {
		var record cacheRecord
		if json.Unmarshal(body, &record) != nil {
			return nil, nil, false
		}
		body, etag = record.Body, record.ETag
	}
	cat, err := Decode(body)
	if err != nil {
		return nil, nil, false
	}
	return &cached{body: body, etag: etag, at: info.ModTime()}, cat, true
}

// writeCache stores body and etag (empty etag stores none).
func writeCache(dir string, body []byte, etag string) {
	if dir == "" || os.MkdirAll(dir, 0o755) != nil {
		return
	}
	data, err := json.Marshal(cacheRecord{Body: body, ETag: etag})
	if err != nil {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	_ = writeAtomic(filepath.Join(dir, recordFile), data)
}

// touchCache marks the cached body as confirmed current just now (a 304).
func touchCache(dir string, now time.Time) {
	_ = os.Chtimes(filepath.Join(dir, recordFile), now, now)
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
