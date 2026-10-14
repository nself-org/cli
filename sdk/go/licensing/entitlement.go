package licensing

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

const FileEnv = "NSELF_ENTITLEMENT_FILE"

// EntitlementGracePeriod is how long past the signed exp a licence JWT still grants a plugin (D8; distinct from DefaultGracePeriod, the CLI offline ladder).
const EntitlementGracePeriod time.Duration = 2592000000000000
const maxTokenBytes = 1048576

var ErrEntitlementExpired = errors.New("entitlement_expired")

// Purpose: verify the configured file. Inputs: plugin slug and trusted clock.
// Outputs: nil only for an authorized signed token. Constraints: absent files fail closed;
// Guard owns the temporary unset-env compatibility rule. SPORT: plugin.entitlement-token.
func VerifyEntitlement(slug string, now time.Time) error {
	return verifyFile(os.Getenv(FileEnv), slug, now)
}

// Purpose: verify a file snapshot. Inputs: path, slug, now. Outputs: authorization.
// Constraints: invalid/unreadable/oversized files fail closed. SPORT: plugin.entitlement-token.
func verifyFile(path, slug string, now time.Time) error {
	if path == "" {
		return ErrEntitlementExpired
	}
	data, err := readTokenFile(path)
	if err != nil {
		return ErrEntitlementExpired
	}
	return verifyToken(strings.TrimSpace(string(data)), slug, now)
}

// Purpose: bound reads of mounted files. Inputs: path. Outputs: bytes or rejection.
// Constraints: oversized files never allocate unbounded memory. SPORT: plugin.entitlement-token.
func readTokenFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrEntitlementExpired
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxTokenBytes+1))
	if err != nil || len(data) > maxTokenBytes {
		return nil, ErrEntitlementExpired
	}
	return data, nil
}

// Purpose: validate compact JWS structure and signature. Inputs: token, slug, now.
// Outputs: authorization. Constraints: EdDSA only, no unsupported critical headers.
// SPORT: plugin.entitlement-token.
func verifyToken(token, slug string, now time.Time) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ErrEntitlementExpired
	}
	enc := base64.RawURLEncoding
	raw, err := enc.DecodeString(parts[0])
	if err != nil {
		return ErrEntitlementExpired
	}
	var header struct {
		Alg  string   `json:"alg"`
		Kid  string   `json:"kid"`
		Crit []string `json:"crit"`
	}
	if json.Unmarshal(raw, &header) != nil || header.Alg != "EdDSA" || len(header.Crit) != 0 {
		return ErrEntitlementExpired
	}
	key := signingKey(header.Kid)
	sig, err := enc.DecodeString(parts[2])
	if len(key) != ed25519.PublicKeySize || err != nil || len(sig) != ed25519.SignatureSize {
		return ErrEntitlementExpired
	}
	if !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), sig) {
		return ErrEntitlementExpired
	}
	payload, err := enc.DecodeString(parts[1])
	if err != nil {
		return ErrEntitlementExpired
	}
	return verifyClaims(payload, slug, now)
}

// Purpose: cache signature and slug authorization. Inputs: token and slug.
// Outputs: signed grace deadline. Constraints: expiry remains checked per request.
// SPORT: plugin.entitlement-token.
func verifiedDeadline(token, slug string) (int64, error) {
	if err := verifyToken(token, slug, time.Unix(0, 0)); err != nil {
		return 0, err
	}
	// The verified compact token and claims are already validated above.
	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	if err != nil {
		return 0, ErrEntitlementExpired
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return 0, ErrEntitlementExpired
	}
	return claims.Exp + int64(EntitlementGracePeriod/time.Second), nil
}

// Purpose: enforce signed claims. Inputs: verified JSON, slug, now.
// Outputs: authorization. Constraints: grace anchors only on signed exp.
// SPORT: plugin.entitlement-token.
func verifyClaims(payload []byte, slug string, now time.Time) error {
	var claims struct {
		Iss     string          `json:"iss"`
		Aud     json.RawMessage `json:"aud"`
		Plugins []string        `json:"plugins"`
		Exp     int64           `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Iss != "ping.nself.org" || !audienceOK(claims.Aud) {
		return ErrEntitlementExpired
	}
	if claims.Exp <= 0 || claims.Exp > (1<<63-1)-int64(EntitlementGracePeriod/time.Second) {
		return ErrEntitlementExpired
	}
	deadline := claims.Exp + int64(EntitlementGracePeriod/time.Second)
	if now.Unix() > deadline || (now.Unix() == deadline && now.Nanosecond() > 0) {
		return ErrEntitlementExpired
	}
	if slug == "" {
		return ErrEntitlementExpired
	}
	for _, allowed := range claims.Plugins {
		if allowed == slug || allowed == "*" {
			return nil
		}
	}
	return ErrEntitlementExpired
}

// Purpose: accept standard JWT audience forms. Inputs: signed audience.
// Outputs: expected audience present. Constraints: types fail closed. SPORT: plugin.entitlement-token.
func audienceOK(raw json.RawMessage) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == "nself-cli"
	}
	var many []string
	if json.Unmarshal(raw, &many) != nil {
		return false
	}
	for _, item := range many {
		if item == "nself-cli" {
			return true
		}
	}
	return false
}
