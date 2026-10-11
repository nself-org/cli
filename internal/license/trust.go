// Package license — trust.go decides whether a cached entry may back a licence
// decision, and says why when it may not.
//
// Purpose: keep a clock refusal apart from a signature refusal. A cache that
// verifies but is read while the clock is behind the evidence (the signed iat,
// or the highest time an earlier check trusted) is refused with text that names
// the clock and the way out, not with the tamper text.
// Inputs: a CacheEntry read from disk; the wall clock.
// Outputs: nil, errCacheUnsigned or errClockBehind.
// Constraints: v1.4 (compat.V15 off) trusts every cache, as before. The
// signature is checked first, so a forged cache never gets the clock text.
package license

import (
	"errors"
	"time"

	"github.com/nself-org/cli/internal/compat"
)

// errCacheUnsigned is the refusal for a cache whose signed body does not verify.
var errCacheUnsigned = errors.New("the cached license is not signed by ping (unsigned, altered or for another key)")

// errClockBehind is the refusal for a verified cache read while the clock is
// behind the signed iat, or more than 10 minutes behind the highest trusted time.
var errClockBehind = errors.New("the system clock is behind the time this license was last checked, so the cached license cannot be trusted; correct the system clock, and if it is already correct, connect to the internet to validate or run 'nself license import <file>' with a fresh export")

// cacheTrust reports whether a cache entry may back a licence decision. Every
// path that reads tier or plugins from the cache calls it, so no decision rests
// on bytes that did not verify. The caller has already matched entry.KeyHash to
// the requested key, and v1.5 verification binds KeyHash to the licence JWT
// inside the signed body. A trusted decision raises the trusted-time mark.
func cacheTrust(entry *CacheEntry) error {
	// compat.V15(P7-PLUG-63): cache trusted without a signature check -> only a cache whose server-signed body verifies
	if !compat.V15() {
		return nil
	}
	now := time.Now()
	if err := entryTrustAt(entry, now); err != nil {
		return err
	}
	noteTrustedTime(now)
	return nil
}

// entryTrustAt is the signature check, then the clock check, at now.
func entryTrustAt(entry *CacheEntry, now time.Time) error {
	if !entry.VerifySignature() {
		return errCacheUnsigned
	}
	if !entry.clockTrusted(now) {
		return errClockBehind
	}
	return nil
}

// cacheRefusalMessage is Validate's user message for a cacheTrust refusal.
func cacheRefusalMessage(err error) string {
	if errors.Is(err, errClockBehind) {
		return "License cache refused: " + err.Error() + "."
	}
	return "License cache is not signed by ping (unsigned, altered or for another key). Connect to the internet to validate."
}

// validatorRefusalReason is the Validator's fail-closed reason for an entryTrustAt refusal.
func validatorRefusalReason(err error) string {
	if errors.Is(err, errClockBehind) {
		return err.Error() + " — refusing fail-open"
	}
	return "license cache signature invalid (tampered or rotated key) — refusing fail-open"
}
