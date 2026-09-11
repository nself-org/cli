// Package license — validator.go implements the FAIL-OPEN license validation
// policy per memory/decisions.md (D3-T10).
//
// Policy summary:
//   - Cache valid + within TTL              → Valid
//   - Cache valid + remote 200 (verified)   → Valid
//   - Cache valid + remote unreachable + age ≤ 72h  → Valid (FAIL-OPEN, silent)
//   - Cache valid + remote unreachable + age 72h-7d → Valid (FAIL-OPEN, warning)
//   - Cache valid + remote unreachable + age > 7d   → FailClosed
//   - Cache signature invalid OR tampered           → FailClosed (NEVER fail-open)
//   - Cache absent + remote unreachable             → FailClosed
//   - Remote 200 with revoked                       → Revoked (overrides cache)
//   - Remote auth failure (401/403)                 → FailClosed (NOT FAIL-OPEN)
//   - Remote transport / 5xx error                  → FAIL-OPEN if cache permits
//
// Network-failure classification distinguishes transport errors from auth
// failures: only transport-class failures qualify for FAIL-OPEN. Auth failures
// indicate explicit policy decisions from the server and must fail-closed.
package license

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ValidatorStatus represents the validator's terminal status decision.
type ValidatorStatus string

const (
	// StatusValid means the license is current and the operation may proceed.
	StatusValid ValidatorStatus = "valid"
	// StatusExpired means the license itself has passed its expires_at.
	StatusExpired ValidatorStatus = "expired"
	// StatusRevoked means the server explicitly revoked the license.
	StatusRevoked ValidatorStatus = "revoked"
	// StatusFailOpen means cache permits proceeding even though the remote was
	// unreachable. CanProceed is true; a stderr warning may be emitted.
	StatusFailOpen ValidatorStatus = "fail_open"
	// StatusFailClosed means the operation must NOT proceed. Bad signature,
	// stale cache beyond hard TTL, missing cache, or auth failure.
	StatusFailClosed ValidatorStatus = "fail_closed"
)

// FailOpenSoftTTL is the silent-fail-open window. ≤ this value, no warning.
// Alias of GraceSoftThreshold (grace.go) — this file no longer declares its
// own value. See grace.go's package comment for why that file owns the
// offline-grace ladder. Configurable for tests via the validator's clock.
const FailOpenSoftTTL = GraceSoftThreshold

// FailOpenHardTTL is the absolute fail-open ceiling. Beyond this, fail-closed.
// Alias of GraceHardThreshold (grace.go). Previously a separate 14-day value;
// unified to the same 7-day ceiling as the rest of the offline ladder
// (P6-E12-W4-S4-T2) — see grace.go's GraceHardThreshold comment for why the
// ceiling does not widen past 7 days.
const FailOpenHardTTL = GraceHardThreshold

// ValidatorResult is the FAIL-OPEN-aware validation outcome.
type ValidatorResult struct {
	Status      ValidatorStatus
	CanProceed  bool
	FromCache   bool
	Tier        string
	Plugins     []string
	CacheAge    time.Duration
	Reason      string
	WarnMessage string // non-empty when caller should emit a stderr warning
}

// Clock abstracts time.Now for testability.
type Clock interface {
	Now() time.Time
}

// realClock returns the wall clock time.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// HTTPDoer abstracts http.Client.Do for testability.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// ValidatorOptions configures Validate.
type ValidatorOptions struct {
	// Clock is used for all time comparisons. nil → wall clock.
	Clock Clock
	// HTTPClient is used for the remote validation call. nil → default 30s client.
	HTTPClient HTTPDoer
	// PingURL overrides the configured ping endpoint. Empty → PingURL().
	PingURL string
	// SkipSignatureVerify allows skipping signature verification for tests.
	// Production callers MUST leave this false.
	SkipSignatureVerify bool
	// WarnOnce is a hook called at most once per process when emitting a
	// FAIL-OPEN warning. nil → default: write to os.Stderr once.
	WarnOnce func(msg string)
}

// failOpenWarnOnce gates the default stderr warning to one emission per process.
//
//nolint:gochecknoglobals // intentional process-lifetime gate
var failOpenWarnOnce sync.Once

func defaultWarnOnce(msg string) {
	failOpenWarnOnce.Do(func() {
		fmt.Fprintln(os.Stderr, msg)
	})
}

// remoteOutcome is an internal enum classifying the outcome of tryRemote.
type remoteOutcome int

const (
	remoteOK remoteOutcome = iota
	remoteAuthFail
	remoteTransientFail
)

// tryRemote performs the HTTP POST to /license/validate and classifies the
// outcome. Returns:
//
//	remoteOK            → server returned 200 (resp parsed)
//	remoteAuthFail      → server returned 401 or 403 (explicit reject; NOT fail-open)
//	remoteTransientFail → transport error or 5xx (fail-open eligible)
func tryRemote(ctx context.Context, key string, opts *ValidatorOptions) (*ValidateResponse, remoteOutcome, error) {
	pingURL := opts.PingURL
	if pingURL == "" {
		pingURL = PingURL()
	}
	type request struct {
		LicenseKey string `json:"license_key"`
		Product    string `json:"product"`
	}
	body, err := json.Marshal(request{LicenseKey: key, Product: "plugins-pro"})
	if err != nil {
		return nil, remoteTransientFail, fmt.Errorf("marshalling request: %w", err)
	}

	url := strings.TrimRight(pingURL, "/") + "/license/validate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, remoteTransientFail, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	var doer = opts.HTTPClient
	if doer == nil {
		doer = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := doer.Do(req)
	if err != nil {
		// Transport error — definitely fail-open eligible.
		return nil, remoteTransientFail, fmt.Errorf("network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain body for connection reuse and to capture potential error details.
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	switch {
	case resp.StatusCode == http.StatusOK:
		// S10.T03: Verify Ed25519 response-body signature before trusting
		// tier/plugins data. Skip in dev builds (IsZeroPubKey) and when
		// SkipSignatureVerify is set in tests.
		if !opts.SkipSignatureVerify && !IsZeroPubKey() {
			sigHex := resp.Header.Get("X-NSelf-License-Sig")
			if sigHex == "" {
				// Missing header — reject, fall through to cache (fail-open eligible).
				return nil, remoteTransientFail, fmt.Errorf("response signature missing (X-NSelf-License-Sig header absent)")
			}
			sigBytes, decErr := hex.DecodeString(sigHex)
			if decErr != nil {
				return nil, remoteTransientFail, fmt.Errorf("response signature malformed: %w", decErr)
			}
			keys := GetPublicKeys()
			var verified bool
			for _, pk := range keys {
				if ed25519.Verify(ed25519.PublicKey(pk.Key), rawBody, sigBytes) {
					verified = true
					break
				}
			}
			if !verified {
				// Tampered or MITM response — reject, fall through to cache.
				return nil, remoteTransientFail, fmt.Errorf("response signature invalid — possible MITM or tampered response")
			}
		}

		var vr ValidateResponse
		if err := json.Unmarshal(rawBody, &vr); err != nil {
			// 200 but unparseable → transient (don't punish user for server bug).
			return nil, remoteTransientFail, fmt.Errorf("decoding response: %w", err)
		}
		return &vr, remoteOK, nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, remoteAuthFail, fmt.Errorf("server returned %d", resp.StatusCode)
	case resp.StatusCode >= 500:
		return nil, remoteTransientFail, fmt.Errorf("server returned %d", resp.StatusCode)
	default:
		// 4xx other than 401/403 — treat as auth-class to avoid silent fail-open
		// on misuse (e.g. 400 bad-request); the caller's cache may not save them.
		// Conservative posture: NOT fail-open.
		return nil, remoteAuthFail, fmt.Errorf("server returned %d", resp.StatusCode)
	}
}
