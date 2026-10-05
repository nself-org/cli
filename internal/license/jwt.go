// Package license — jwt.go verifies the licence JWT ping_api issues in the
// `jwt` field of POST /license/validate.
//
// Purpose: let the CLI check a licence offline against the committed ping
// public keys (PingKeys) with the standard library only (no JWT library).
// Format (web backend/services/ping_api/src/license/jwt.ts): EdDSA, header
// {alg, typ, kid}, claims {sub = sha256(licence key), tier, plugins, iat, exp
// (24 h), iss "ping.nself.org", aud "nself-cli", revocable, renewal}.
// Constraints: only alg EdDSA with a kid found in PingKeys is accepted. exp is
// returned, not enforced here: the offline grace ladder (grace.go) owns time
// decisions and never trusts a locally stamped time as a signature anchor.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// jwtIssuer and jwtAudience are what ping_api puts in every licence JWT.
	jwtIssuer   = "ping.nself.org"
	jwtAudience = "nself-cli"
)

// ErrInvalidLicenseJWT wraps every ParseLicenseJWT rejection.
var ErrInvalidLicenseJWT = errors.New("invalid licence token")

// LicenseClaims are the verified claims of a licence JWT.
type LicenseClaims struct {
	Sub       string   // sha256 hex of the licence key
	Tier      string   // ping's tier label
	Plugins   []string // plugins the licence covers; ["*"] means all
	Iat       int64
	Exp       int64
	Renewal   string // RFC 3339, empty when none
	Revocable bool
}

// Expired reports whether the token's exp is at or before nowUnix. The caller
// supplies the time; this package keeps no clock of its own for it.
func (c *LicenseClaims) Expired(nowUnix int64) bool { return c.Exp <= nowUnix }

// AllowedPlugins returns the plugin list ping sent: plugins_allowed, falling
// back to the legacy `plugins` field only when plugins_allowed is absent.
func (r *ValidateResponse) AllowedPlugins() []string {
	if r.PluginsAllowed != nil {
		return r.PluginsAllowed
	}
	return r.Plugins
}

// pubKeyFromHex decodes a 32-byte Ed25519 public key; nil when malformed.
func pubKeyFromHex(h string) ed25519.PublicKey {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}

func jwtErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidLicenseJWT, fmt.Sprintf(format, a...))
}

// ParseLicenseJWT verifies token and returns its claims.
//
// It rejects, with an error wrapping ErrInvalidLicenseJWT: anything that is
// not three base64url segments, an alg other than EdDSA (none, HS256, ...), a
// `crit` header, a kid that is not in PingKeys, a signature that does not
// verify over "header.payload" with that kid's key, and an iss or aud other
// than ping.nself.org / nself-cli. The signature is checked before the claims
// are decoded.
func ParseLicenseJWT(token string) (*LicenseClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, jwtErr("expected three segments")
	}
	enc := base64.RawURLEncoding
	hdrRaw, err := enc.DecodeString(parts[0])
	if err != nil {
		return nil, jwtErr("header is not base64url")
	}
	var hdr struct {
		Alg  string   `json:"alg"`
		Kid  string   `json:"kid"`
		Crit []string `json:"crit"`
	}
	if err := json.Unmarshal(hdrRaw, &hdr); err != nil {
		return nil, jwtErr("header is not JSON")
	}
	if hdr.Alg != "EdDSA" {
		return nil, jwtErr("alg %q is not accepted", hdr.Alg)
	}
	if len(hdr.Crit) > 0 {
		return nil, jwtErr("crit header is not supported")
	}
	var pub ed25519.PublicKey
	for _, k := range PingKeys {
		if k.ID != "" && k.ID == hdr.Kid && len(k.Public) == ed25519.PublicKeySize {
			pub = k.Public
			break
		}
	}
	if pub == nil {
		return nil, jwtErr("unknown kid %q", hdr.Kid)
	}
	sig, err := enc.DecodeString(parts[2])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, jwtErr("signature is malformed")
	}
	if !ed25519.Verify(pub, []byte(parts[0]+"."+parts[1]), sig) {
		return nil, jwtErr("signature does not verify")
	}
	payload, err := enc.DecodeString(parts[1])
	if err != nil {
		return nil, jwtErr("payload is not base64url")
	}
	var c struct {
		Sub       string          `json:"sub"`
		Tier      string          `json:"tier"`
		Plugins   []string        `json:"plugins"`
		Iat       int64           `json:"iat"`
		Exp       int64           `json:"exp"`
		Iss       string          `json:"iss"`
		Aud       json.RawMessage `json:"aud"`
		Renewal   string          `json:"renewal"`
		Revocable bool            `json:"revocable"`
	}
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, jwtErr("payload is not JSON")
	}
	if c.Iss != jwtIssuer {
		return nil, jwtErr("issuer %q is not accepted", c.Iss)
	}
	if !audienceIncludes(c.Aud, jwtAudience) {
		return nil, jwtErr("audience does not include %s", jwtAudience)
	}
	return &LicenseClaims{Sub: c.Sub, Tier: c.Tier, Plugins: c.Plugins, Iat: c.Iat, Exp: c.Exp,
		Renewal: c.Renewal, Revocable: c.Revocable}, nil
}

// audienceIncludes accepts aud as a string or an array of strings.
func audienceIncludes(raw json.RawMessage, want string) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == want
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, a := range many {
			if a == want {
				return true
			}
		}
	}
	return false
}
