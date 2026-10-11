package license

// revocation_refresh_check.go — fetching the revocation list and checking a license.
//
// Purpose: fetch the signed revocation list from ping.nself.org, and check whether a given license record is revoked under the fail-open staleness policy, split out of revocation.go for file size.
// Inputs: a LicenseRecord to check, or nothing for a background refresh.
// Outputs: a revoked/not-revoked decision, or an updated on-disk revocation cache.
// Constraints: pure move from revocation.go (CLI-R12 Batch F); no behaviour change. FAIL-OPEN policy (cache >7 days stale + fetch fails => treat as not revoked, warn) is preserved exactly.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"strings"
	"sync"
	"time"

	saferecover "github.com/nself-org/cli/internal/recover"
)

// revocationHTTPClient is shared so transports get reused.
var revocationHTTPClient = &http.Client{Timeout: revocationHTTPTimeout}

// SetRevocationHTTPClient overrides the HTTP client used for refreshes.
// Test-only.
func SetRevocationHTTPClient(c *http.Client) {
	revocationHTTPClient = c
}

// RefreshRevocationList fetches the signed list from ping.nself.org,
// verifies the signature, and persists it on disk.
//
// Behaviour:
//   - Network / decode error → returns the wrapped error; does NOT touch disk.
//   - Bad signature → returns ErrRevocationSignatureInvalid; does NOT persist.
//   - Success → writes cache and returns the new RevocationCache.
func RefreshRevocationList(ctx context.Context) (*RevocationCache, error) {
	url := strings.TrimRight(PingURL(), "/") + "/license/revocation-list"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	// Send conditional GET when we have a stored cache + ETag to allow 304s.
	// The server (revocation-list.ts) honours If-None-Match against a strong
	// sha256-hex ETag; If-Modified-Since is NOT honoured. Cold-start caches
	// (no ETag yet) simply skip the conditional header and pull a full 200.
	if existing, _ := ReadRevocationCache(); existing != nil && existing.ETag != "" {
		req.Header.Set("If-None-Match", existing.ETag)
	}

	resp, err := revocationHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching revocation list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		// Server confirms cache is current; touch FetchedAt so the
		// staleness window resets even though contents didn't change.
		existing, _ := ReadRevocationCache()
		if existing != nil {
			existing.FetchedAt = time.Now().Unix()
			if err := WriteRevocationCache(existing); err != nil {
				return nil, err
			}
			return existing, nil
		}
		// 304 with no local cache is contractually impossible but
		// graceful enough — fall through to a full fetch attempt
		// next call.
		return nil, fmt.Errorf("304 received with no local cache")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf(
			"revocation list HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading revocation list body: %w", err)
	}

	var list RevocationList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parsing revocation list: %w", err)
	}

	if !VerifyRevocationSignature(&list) {
		// Skip verification only when the build has no signing key AND
		// no test override is in effect.  Production builds always
		// verify; test runs with LICENSE_PUBLIC_KEY_OVERRIDE always
		// verify.
		if !IsZeroPubKey() || os.Getenv("LICENSE_PUBLIC_KEY_OVERRIDE") != "" {
			return nil, ErrRevocationSignatureInvalid
		}
	}

	cache := &RevocationCache{
		List:      list,
		FetchedAt: time.Now().Unix(),
		ETag:      resp.Header.Get("ETag"),
	}
	if err := WriteRevocationCache(cache); err != nil {
		return nil, fmt.Errorf("persisting revocation cache: %w", err)
	}
	return cache, nil
}

// ErrRevocationSignatureInvalid signals that a fetched payload failed
// signature verification.  Callers should treat this as a refresh failure
// and keep the existing cache untouched.
var ErrRevocationSignatureInvalid = fmt.Errorf("revocation list signature invalid")

// ─── Lookup (FAIL-OPEN) ──────────────────────────────────────────────────────

// IsRecordRevoked reports whether the supplied license record is on the
// cached revocation list.  Implements the FAIL-OPEN policy described at
// the top of this file.
//
// Returns false (not revoked) when:
//   - the cache file is absent (cold start);
//   - the cache is older than RevocationFailOpenStaleness (7 days) AND
//     the most recent refresh attempt failed;
//   - the cached list does not contain a matching identifier.
//
// A prominent warning is printed to stderr (once per process) when the
// cache is fail-open-stale.
//
// Disambiguation: this is distinct from the legacy IsRevoked() in
// revoke.go, which reports whether the local "license.revoked" marker
// file is present (the marker is an out-of-band signal written by
// `nself license revoke` for plugin dormancy).
func IsRecordRevoked(rec LicenseRecord) bool {
	cache, err := ReadRevocationCache()
	if err != nil || cache == nil {
		return false
	}
	if isStaleFailOpen(cache, time.Now()) {
		warnFailOpenOnce(cache)
		return false
	}
	return matchAny(&cache.List, rec)
}

// matchAny scans the revocation list for any entry matching the record.
func matchAny(list *RevocationList, rec LicenseRecord) bool {
	for i := range list.Revoked {
		e := &list.Revoked[i]
		switch e.Type {
		case "jti":
			if rec.JTI != "" && e.ID == rec.JTI {
				return true
			}
		case "user_id":
			if rec.UserID != "" && e.ID == rec.UserID {
				return true
			}
		case "kid":
			if rec.Kid != "" && e.ID == rec.Kid {
				return true
			}
		case "key_hash":
			if rec.KeyHash != "" && e.ID == rec.KeyHash {
				return true
			}
		}
	}
	return false
}

// isStaleFailOpen reports whether the cache is past the FAIL-OPEN cutoff.
func isStaleFailOpen(cache *RevocationCache, now time.Time) bool {
	if cache == nil {
		return true
	}
	return now.Sub(time.Unix(cache.FetchedAt, 0)) > RevocationFailOpenStaleness
}

// ─── Stale-warning suppression (one log per process) ────────────────────────

var (
	failOpenWarnedMu sync.Mutex
	failOpenWarned   bool
)

// warnFailOpenOnce prints the prominent stderr warning at most once per
// CLI invocation.
func warnFailOpenOnce(cache *RevocationCache) {
	failOpenWarnedMu.Lock()
	defer failOpenWarnedMu.Unlock()
	if failOpenWarned {
		return
	}
	failOpenWarned = true
	age := time.Since(time.Unix(cache.FetchedAt, 0)).Round(time.Hour)
	fmt.Fprintf(os.Stderr,
		"warning: revocation list cache is %s stale (>7d) and last refresh failed; treating licenses as not revoked\n",
		age,
	)
}

// ResetFailOpenWarning clears the once-per-process warning latch.  Test-only.
func ResetFailOpenWarning() {
	failOpenWarnedMu.Lock()
	failOpenWarned = false
	failOpenWarnedMu.Unlock()
}

// ─── Background refresher ───────────────────────────────────────────────────

// StartRevocationRefresher launches a background goroutine that calls
// RefreshRevocationList every RevocationRefreshInterval until ctx is done.
// Refresh failures are logged but do not stop the loop — the FAIL-OPEN
// policy is enforced at lookup time, not here.
//
// Returns a stop function that the caller can defer to halt the loop
// before ctx is cancelled.
func StartRevocationRefresher(ctx context.Context) (stop func()) {
	loopCtx, cancel := context.WithCancel(ctx)
	saferecover.SafeGo("license_revocation_refresh", func() {
		// Initial best-effort refresh on startup.
		refreshAndWarn(loopCtx, os.Stderr, "initial revocation refresh")
		ticker := time.NewTicker(RevocationRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				refreshAndWarn(loopCtx, os.Stderr, "revocation refresh")
			}
		}
	})
	return cancel
}

// refreshAndWarn runs one refresh and reports a failure on w as "warning: <what> failed".
// A failure never stops the refresher: FAIL-OPEN is decided at lookup time.
func refreshAndWarn(ctx context.Context, w io.Writer, what string) {
	if _, err := RefreshRevocationList(ctx); err != nil {
		fmt.Fprintf(w, "warning: %s failed: %v\n", what, err)
	}
}
