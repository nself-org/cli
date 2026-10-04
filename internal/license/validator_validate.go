package license

// Purpose: the top-level Validate entrypoint implementing the FAIL-OPEN license validation policy (remote-first, cache-fallback with soft/hard TTL) documented in validator.go's package doc.
// Inputs: a license key (may be empty for cache-only validation) and ValidatorOptions (HTTP client, cache path, clock).
// Outputs: a ValidatorResult describing Valid/Revoked/FailOpen/FailClosed, or an error.
// Constraints: split out of validator.go as a pure move (CLI-R12); no behavior change. Never change fail-open/fail-closed classification without updating memory/decisions.md D3-T10.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Validate executes the FAIL-OPEN license validation flow.
//
// Steps:
//  1. Try remote validation (when HTTPClient + key supplied). Distinguish:
//     - 200 valid:           update cache, return Valid
//     - 200 invalid/revoked: return Revoked
//     - 401/403:             return FailClosed (auth failure, NOT fail-open)
//     - 5xx / transport err: fall through to cache-only path (fail-open eligible)
//  2. Cache-only path:
//     - cache absent:         FailClosed
//     - signature invalid:    FailClosed (never fail-open on tamper)
//     - cache age ≤ soft TTL: FailOpen (silent — no warning)
//     - cache age ≤ hard TTL: FailOpen (warning)
//     - cache age > hard TTL: FailClosed
//
// `key` may be empty to validate from cache only (no remote call).
func Validate(ctx context.Context, key string, opts *ValidatorOptions) (*ValidatorResult, error) {
	if opts == nil {
		opts = &ValidatorOptions{}
	}
	clk := opts.Clock
	if clk == nil {
		clk = realClock{}
	}
	warnFn := opts.WarnOnce
	if warnFn == nil {
		warnFn = defaultWarnOnce
	}

	// 1) Try remote — but only if a key is supplied.
	if key != "" {
		remote, kind, err := tryRemote(ctx, key, opts)
		switch kind {
		case remoteOK:
			// Persist successful response to cache.
			entry := responseToCache(key, remote)
			if writeErr := WriteCache(entry); writeErr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not update license cache: %v\n", writeErr)
			}
			if !remote.Valid {
				// Server says revoked/expired. This OVERRIDES cache.
				status := StatusRevoked
				reason := remote.Reason
				if reason == "" {
					reason = "license rejected by server"
				}
				if strings.Contains(strings.ToLower(reason), "expired") {
					status = StatusExpired
				}
				return &ValidatorResult{
					Status:     status,
					CanProceed: false,
					FromCache:  false,
					Reason:     reason,
				}, nil
			}
			return &ValidatorResult{
				Status:     StatusValid,
				CanProceed: true,
				FromCache:  false,
				Tier:       remote.Tier,
				Plugins:    pluginsFor(remote),
			}, nil
		case remoteAuthFail:
			// 401/403 — explicit auth rejection. NEVER fail-open here.
			return &ValidatorResult{
				Status:     StatusFailClosed,
				CanProceed: false,
				FromCache:  false,
				Reason:     fmt.Sprintf("license rejected by server (auth): %v", err),
			}, nil
		case remoteTransientFail:
			// transport error / 5xx — fall through to fail-open eligibility check
		}
	}

	// 2) Cache-only path.
	entry, cacheErr := ReadCache()
	if cacheErr != nil || entry == nil {
		return &ValidatorResult{
			Status:     StatusFailClosed,
			CanProceed: false,
			FromCache:  false,
			Reason:     "no license cache available and remote unreachable",
		}, nil
	}

	// Verify signature unless explicitly skipped (tests).
	if !opts.SkipSignatureVerify && !IsZeroPubKey() && !entry.VerifySignature() {
		return &ValidatorResult{
			Status:     StatusFailClosed,
			CanProceed: false,
			FromCache:  true,
			Reason:     "license cache signature invalid (tampered or rotated key) — refusing fail-open",
		}, nil
	}

	// If a key was supplied, ensure the cache matches.
	if key != "" && entry.KeyHash != HashKey(key) {
		return &ValidatorResult{
			Status:     StatusFailClosed,
			CanProceed: false,
			FromCache:  true,
			Reason:     "license cache does not match supplied key",
		}, nil
	}

	// Check server-reported expiry first.
	if entry.ExpiresAt > 0 {
		expiresAt := time.Unix(entry.ExpiresAt, 0)
		if clk.Now().After(expiresAt) {
			return &ValidatorResult{
				Status:     StatusExpired,
				CanProceed: false,
				FromCache:  true,
				Tier:       entry.Tier,
				Reason:     fmt.Sprintf("license expired on %s", expiresAt.Format("2006-01-02")),
			}, nil
		}
	}

	age := clk.Now().Sub(time.Unix(entry.FetchedAt, 0))

	// BUILD-LEDGER Finding #14: the local revocation cache is the
	// compensating control that makes FAIL-OPEN safe. Consult it
	// unconditionally before either fail-open branch below can grant
	// CanProceed: true — a revoked license must never ride the soft or
	// hard TTL window just because the remote validator is unreachable.
	// KeyHash (not JTI) is the join key here: CacheEntry never carries a
	// JTI (see LicenseRecord.KeyHash doc comment in revocation.go).
	if IsRecordRevoked(LicenseRecord{KeyHash: entry.KeyHash}) {
		return &ValidatorResult{
			Status:     StatusFailClosed,
			CanProceed: false,
			FromCache:  true,
			Tier:       entry.Tier,
			Reason:     "license revoked; cannot fail-open a revoked license",
		}, nil
	}

	switch {
	case age <= FailOpenSoftTTL:
		return &ValidatorResult{
			Status:     StatusFailOpen,
			CanProceed: true,
			FromCache:  true,
			Tier:       entry.Tier,
			Plugins:    entry.PluginsAllowed,
			CacheAge:   age,
			Reason:     "remote unreachable; using cached license within soft TTL",
		}, nil
	case age <= FailOpenHardTTL:
		warn := fmt.Sprintf(
			"WARNING: nSelf license validation has been offline for %s. "+
				"Connect to the internet within %s or paid plugins will go dormant.",
			formatDuration(age),
			formatDuration(FailOpenHardTTL-age),
		)
		warnFn(warn)
		return &ValidatorResult{
			Status:      StatusFailOpen,
			CanProceed:  true,
			FromCache:   true,
			Tier:        entry.Tier,
			Plugins:     entry.PluginsAllowed,
			CacheAge:    age,
			Reason:      "remote unreachable; using cached license within hard TTL (warning issued)",
			WarnMessage: warn,
		}, nil
	default:
		return &ValidatorResult{
			Status:     StatusFailClosed,
			CanProceed: false,
			FromCache:  true,
			Tier:       entry.Tier,
			CacheAge:   age,
			Reason: fmt.Sprintf(
				"license cache is %s old, exceeding %s fail-open ceiling — refusing to proceed",
				formatDuration(age), formatDuration(FailOpenHardTTL),
			),
		}, nil
	}
}
