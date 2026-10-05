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
		return entry.VerifySignature()
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
