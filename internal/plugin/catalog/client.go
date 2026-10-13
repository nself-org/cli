package catalog

// client.go — fetch the catalog with an ETag cache and an offline fallback.
//
// Purpose: one GET of the catalog per command, revalidated with If-None-Match
// so an unchanged catalog costs a 304 and no body.
// Inputs: ctx, the catalog URL and the cache dir. Outputs: a Result.
// Constraints: the HTTP client is a copy of httptimeout.Plugin (15 s); a
// non-HTTPS URL is refused unless it is loopback; redirects may not leave HTTPS
// (or loopback). A body that does not decode is E116 and never replaces the
// cache. When the network fails, the cache answers (Source cache); with no
// cache the error is E115.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
)

// DefaultURL is the served catalog (contract:plugins.catalog v1).
const DefaultURL = "https://plugins.nself.org/catalog.json"

// Source says where a Result came from.
type Source string

const (
	// SourceNetwork: the server answered (200, or 304 confirming the cache).
	SourceNetwork Source = "network"
	// SourceCache: the server could not be reached; the stored copy answered.
	SourceCache Source = "cache"
)

// Result is a decoded catalog and its provenance. FetchedAt is when the body
// was last confirmed current (now for the network, the cache mtime otherwise).
type Result struct {
	Catalog   *Catalog
	Source    Source
	FetchedAt time.Time
}

// Note is the stderr line to print for a cache-served result, "" otherwise.
func (r Result) Note() string {
	if r.Source != SourceCache {
		return ""
	}
	return "using cached catalog from " + r.FetchedAt.Format("2006-01-02")
}

// newClient returns the catalog HTTP client: httptimeout.Plugin's timeout with
// a redirect guard.
func newClient() *http.Client {
	c := *httptimeout.Plugin
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after %d redirects", len(via))
		}
		return checkURL(req.URL)
	}
	return &c
}

// checkURL allows https, and plain http only to a loopback host (dev, tests).
func checkURL(u *url.URL) error {
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		h := u.Hostname()
		if h == "localhost" || h == "127.0.0.1" || h == "::1" {
			return nil
		}
	}
	return fmt.Errorf("refusing non-HTTPS catalog URL %q", u.Redacted())
}

// Fetch returns the catalog from url, revalidating the cache in cacheDir.
func Fetch(ctx context.Context, rawURL, cacheDir string) (Result, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Result{}, unavailable("bad catalog URL: %v", err)
	}
	if err := checkURL(u); err != nil {
		return Result{}, unavailable("%v", err)
	}
	stored, storedCat, haveCache := readCache(cacheDir)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, unavailable("%v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nself-cli")
	if haveCache && stored.etag != "" {
		req.Header.Set("If-None-Match", stored.etag)
	}

	resp, err := newClient().Do(req)
	if err != nil {
		return fromCache(haveCache, stored, storedCat, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body

	switch {
	case resp.StatusCode == http.StatusNotModified && haveCache:
		now := time.Now()
		touchCache(cacheDir, now)
		return Result{Catalog: storedCat, Source: SourceNetwork, FetchedAt: now}, nil
	case resp.StatusCode == http.StatusOK:
		body, err := readBody(resp.Body)
		if err != nil {
			var coded *errs.CLIError
			if errors.As(err, &coded) {
				return Result{}, err
			}
			return fromCache(haveCache, stored, storedCat, err)
		}
		cat, err := Decode(body)
		if err != nil {
			return Result{}, err
		}
		writeCache(cacheDir, body, strings.TrimSpace(resp.Header.Get("ETag")))
		return Result{Catalog: cat, Source: SourceNetwork, FetchedAt: time.Now()}, nil
	default:
		return fromCache(haveCache, stored, storedCat, fmt.Errorf("%s answered %s", u.Host, resp.Status))
	}
}

// fromCache answers from the stored copy, or reports E115 when there is none.
func fromCache(have bool, c *cached, cat *Catalog, cause error) (Result, error) {
	if !have {
		return Result{}, unavailable("%v, and nothing is cached yet", cause)
	}
	return Result{Catalog: cat, Source: SourceCache, FetchedAt: c.at}, nil
}
