// Package license — cache_entry.go holds the licence cache entry and the
// checks that decide whether a cached entry can be trusted.
//
// Purpose: store what ping_api signed (the raw /license/validate body, its
// X-NSelf-License-Sig header and the licence JWT) and verify the cache against
// that server signature, never against a value the CLI stamped itself.
// Inputs: a ValidateResponse from validateRemote (RawBody, BodySig).
// Outputs: CacheEntry values and VerifySignature / pluginsFor decisions.
// Constraints: capturing the new fields is additive in both compat modes;
// VerifySignature and pluginsFor switch to the corrected data only under
// compat.V15() (ADR 0021). Fail closed: a present-but-invalid signature never
// verifies, an entry with no signed body never verifies in v1.5.
package license

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
)

// CacheEntry is one cached licence validation.
//
// RawBody, BodySig, JWT and JWTKid come from ping_api and are what v1.5 trusts.
// Tier, PluginsAllowed and ExpiresAt are copies of fields inside RawBody that
// callers read; v1.5 verification rejects an entry whose copies differ from
// the signed body. FetchedAt is the local clock when the response arrived: it
// is informational (cache age) and never part of anything signed.
// Signature and SignatureKeyID are the legacy fields no server ever filled;
// they are read from old cache files and verified only in v1.4 mode.
type CacheEntry struct {
	KeyHash        string   `json:"key_hash"`
	Tier           string   `json:"tier"`
	PluginsAllowed []string `json:"plugins_allowed"`
	FetchedAt      int64    `json:"fetched_at"`
	ExpiresAt      int64    `json:"expires_at"`
	RawBody        string   `json:"raw_body,omitempty"`
	BodySig        string   `json:"body_sig,omitempty"`
	JWT            string   `json:"jwt,omitempty"`
	JWTKid         string   `json:"jwt_kid,omitempty"`
	Signature      string   `json:"signature,omitempty"`
	SignatureKeyID int      `json:"signature_key_id,omitempty"`
}

// pluginsFor returns the plugin list a decision should use for resp.
func pluginsFor(resp *ValidateResponse) []string {
	// compat.V15(P7-PLUG-63): plugins field ping never sends (always empty) -> plugins_allowed
	if compat.V15() {
		return resp.AllowedPlugins()
	}
	return resp.Plugins
}

// VerifySignature reports whether the entry can be trusted.
//
// v1.5: the entry holds the raw body ping signed, BodySig verifies over those
// exact bytes with a committed PingKeys key, and the entry's copies (tier,
// plugins, expiry, jwt) match that signed body and the signed JWT's sub is the
// entry's key hash.
// v1.4: the legacy check over the locally built payload (never true for an
// entry this CLI wrote, since no server fills Signature).
func (c *CacheEntry) VerifySignature() bool {
	// compat.V15(P7-PLUG-63): signature over a locally built payload -> server signature over the raw body
	if compat.V15() {
		return c.verifySignedBody()
	}
	return c.verifyLegacy()
}

// verifySignedBody is the v1.5 check.
func (c *CacheEntry) verifySignedBody() bool {
	if c.RawBody == "" || c.BodySig == "" {
		return false
	}
	sig, err := hex.DecodeString(c.BodySig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	ok := false
	for _, k := range PingKeys {
		if len(k.Public) == ed25519.PublicKeySize && ed25519.Verify(k.Public, []byte(c.RawBody), sig) {
			ok = true
			break
		}
	}
	return ok && c.matchesSignedBody()
}

// matchesSignedBody checks the copies callers read against the signed body.
func (c *CacheEntry) matchesSignedBody() bool {
	var body ValidateResponse
	if err := json.Unmarshal([]byte(c.RawBody), &body); err != nil || !body.Valid {
		return false
	}
	var exp int64
	if body.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, body.ExpiresAt)
		if err != nil {
			return false
		}
		exp = t.Unix()
	}
	if c.Tier != body.Tier || c.ExpiresAt != exp || !slices.Equal(c.PluginsAllowed, body.AllowedPlugins()) {
		return false
	}
	// The licence identity comes from the JWT inside the signed body, never from
	// the cache's own unsigned copy: a blanked or swapped copy must not unbind
	// the entry from its licence (one leaked signed cache would unlock any key).
	if body.JWT == "" || c.JWT != body.JWT {
		return false
	}
	claims, err := ParseLicenseJWT(body.JWT)
	return err == nil && claims.Sub == c.KeyHash
}

// cacheSignatureOK reports whether a cache entry may back a licence decision.
// Every path that reads tier or plugins from the cache calls it, so no decision
// rests on bytes that did not verify. The caller has already matched
// entry.KeyHash to the requested key, and v1.5 verification binds KeyHash to
// the licence JWT inside the signed body.
func cacheSignatureOK(entry *CacheEntry) bool {
	// compat.V15(P7-PLUG-63): cache trusted without a signature check -> only a cache whose server-signed body verifies
	if compat.V15() {
		now := time.Now()
		if !entry.VerifySignature() || !entry.clockTrusted(now) {
			return false
		}
		noteTrustedTime(now)
	}
	return true
}

// verifyLegacy is the v1.4 check, unchanged.
func (c *CacheEntry) verifyLegacy() bool {
	keys := GetPublicKeys()
	for _, pk := range keys {
		if c.SignatureKeyID != 0 && pk.ID != c.SignatureKeyID {
			continue
		}
		sigBytes, err := hex.DecodeString(c.Signature)
		if err != nil {
			continue
		}
		if ed25519.Verify(pk.Key, c.signablePayload(), sigBytes) {
			return true
		}
	}
	// If keyID was specified and didn't match, try all keys (rotation window).
	if c.SignatureKeyID != 0 {
		sigBytes, err := hex.DecodeString(c.Signature)
		if err != nil {
			return false
		}
		payload := c.signablePayload()
		for _, pk := range keys {
			if ed25519.Verify(pk.Key, payload, sigBytes) {
				return true
			}
		}
	}
	return false
}

// signablePayload is the v1.4 legacy payload. It includes the locally stamped
// fetched_at, which no server can sign; v1.5 does not use it.
//
// PluginsAllowed is sorted alphabetically and joined with commas so cached
// plugin names cannot be injected without breaking the signature (SIEGE V03-F01).
func (c *CacheEntry) signablePayload() []byte {
	sorted := make([]string, len(c.PluginsAllowed))
	copy(sorted, c.PluginsAllowed)
	sort.Strings(sorted)
	return []byte(fmt.Sprintf("%s|%s|%d|%d|%s",
		c.KeyHash, c.Tier, c.FetchedAt, c.ExpiresAt, strings.Join(sorted, ",")))
}

// verifyResponseSig checks the X-NSelf-License-Sig header over the raw body
// against the active public keys (GetPublicKeys). Errors say the caller falls
// back to the cache, so a bad signature is never read as a valid response.
func verifyResponseSig(rawBody []byte, sigHex string) error {
	if sigHex == "" {
		return fmt.Errorf("response signature missing (X-NSelf-License-Sig header absent) — falling back to cache")
	}
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return fmt.Errorf("response signature malformed: %w — falling back to cache", err)
	}
	for _, pk := range GetPublicKeys() {
		if ed25519.Verify(pk.Key, rawBody, sigBytes) {
			return nil
		}
	}
	return fmt.Errorf("response signature invalid — possible MITM or tampered response; falling back to cache")
}

// replySkewSec is the clock skew allowed on a reply's issued-at. A reply may
// claim a validity window of at most 24 h (ping's licence JWT TTL).
const replySkewSec = 300

// replyBoundToKey is the v1.5 check on a verified /license/validate reply: its
// signed bytes must name the requesting licence and be inside their validity
// window. The licence JWT inside the signed body is the only such field ping
// signs today: sub = sha256(key), iat, exp. Without it nothing says which
// licence the reply is for, so a genuine reply for another key (replayed via
// LICENSE_PING_URL or a proxy) would grant here. Fail closed.
func replyBoundToKey(resp *ValidateResponse, key string, now time.Time) error {
	if resp.JWT == "" {
		return fmt.Errorf("signed reply carries no licence jwt, so it is not bound to this licence")
	}
	claims, err := ParseLicenseJWT(resp.JWT)
	if err != nil {
		return fmt.Errorf("signed reply's licence jwt is invalid: %w", err)
	}
	if claims.Sub != HashKey(key) {
		return fmt.Errorf("signed reply is for a different licence key")
	}
	if claims.Iat > now.Unix()+replySkewSec || claims.Exp <= now.Unix() {
		return fmt.Errorf("signed reply is outside its validity window (replayed or clock skew)")
	}
	return nil
}

// checkReplyBinding applies replyBoundToKey to a valid reply from v1.5 on.
func checkReplyBinding(resp *ValidateResponse, key string, now time.Time) error {
	// compat.V15(P7-PLUG-63): signed reply trusted for any licence -> only a reply whose signed jwt names this key and is in window
	if compat.V15() && resp.Valid {
		return replyBoundToKey(resp, key, now)
	}
	return nil
}

// checkBundleBinding applies bundleReplyBound to a valid bundle reply from v1.5 on.
func checkBundleBinding(r *bundleValidateResponse, key, bundle string, now time.Time) error {
	// compat.V15(P7-PLUG-63): bundle reply trusted for any key and bundle -> only a signed reply naming this key, this bundle and a live window
	if compat.V15() && r.Valid {
		return bundleReplyBound(r, key, bundle, now)
	}
	return nil
}

// bundleReplyBound is the v1.5 check on a verified /license/validate?bundle=
// reply: the signed bytes must name this licence (key_hash = sha256 of the key),
// this bundle, and a validity window that contains now.
func bundleReplyBound(r *bundleValidateResponse, key, bundle string, now time.Time) error {
	if r.Bundle != bundle {
		return fmt.Errorf("signed reply is for bundle %q, not %q", r.Bundle, bundle)
	}
	if r.KeyHash != HashKey(key) {
		return fmt.Errorf("signed reply does not name this licence key (key_hash)")
	}
	if r.IssuedAt <= 0 || r.ExpiresAt <= r.IssuedAt || r.ExpiresAt-r.IssuedAt > int64(24*time.Hour/time.Second) {
		return fmt.Errorf("signed reply carries no usable issued_at/expires_at window")
	}
	if r.IssuedAt > now.Unix()+replySkewSec || r.ExpiresAt <= now.Unix() {
		return fmt.Errorf("signed reply is outside its validity window (replayed or clock skew)")
	}
	return nil
}

// signedIssuedAt returns the iat of the licence JWT inside the signed body.
func (c *CacheEntry) signedIssuedAt() (int64, bool) {
	var body ValidateResponse
	if json.Unmarshal([]byte(c.RawBody), &body) != nil || body.JWT == "" {
		return 0, false
	}
	claims, err := ParseLicenseJWT(body.JWT)
	if err != nil {
		return 0, false
	}
	return claims.Iat, true
}

// AgeAt is how old the cached licence is at now. v1.4: since the locally
// stamped fetched_at. v1.5: the older of that and the signed JWT iat, so
// restamping fetched_at cannot keep a once-genuine entry fresh forever.
func (c *CacheEntry) AgeAt(now time.Time) time.Duration {
	age := now.Sub(time.Unix(c.FetchedAt, 0))
	// compat.V15(P7-PLUG-63): cache age from the unsigned fetched_at -> the older of fetched_at and the signed jwt iat
	if compat.V15() {
		if iat, ok := c.signedIssuedAt(); ok {
			if signed := now.Sub(time.Unix(iat, 0)); signed > age {
				age = signed
			}
		}
	}
	return age
}
