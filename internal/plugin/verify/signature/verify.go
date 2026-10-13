package signature

// verify.go — checks one registry release_signature against the compiled-in keys.
//
// Purpose: verify the release_signature of a registry or catalog entry over the domain-separated string of hq PLUG D7, for one tier.
// Inputs: tier, slug, version, the lowercase hex sha256 of the signed tarball, and the entry's release_signature.
// Outputs: nil, or a *errs.CLIError: E121 bad or malformed signature, E122 unknown key (also a key of the other tier), E123 revoked key.
// Constraints: pure (no I/O, no clock). Fail closed: any problem, including a broken key table, is an error. The checksum compare (E124) happens before this, in the installer.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// Alg is the only signature algorithm the contract defines.
const Alg = "ed25519"

// Release is the release_signature object of a registry or catalog entry.
type Release struct {
	KeyID string `json:"key_id"`
	Alg   string `json:"alg"`
	Sig   string `json:"sig"` // raw 64-byte signature, standard padded base64
}

// SigningInput returns the bytes the release pipeline signs:
// "nself-plugin-v1\n<slug>\n<version>\n<sha256 hex>\n".
func SigningInput(slug, version, sha256hex string) []byte {
	return []byte("nself-plugin-v1\n" + slug + "\n" + version + "\n" + sha256hex + "\n")
}

// cleanField reports whether s can sit in the signing input without changing
// its line structure.
func cleanField(s string) bool {
	return s != "" && !strings.ContainsAny(s, "\n\r\x00")
}

// isLowerHex64 reports whether s is a 64-character lowercase hex string.
func isLowerHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Verify checks rel for the given tier, slug, version and tarball sha256.
func Verify(tier, slug, version, sha256hex string, rel *Release) error {
	if rel == nil {
		return fail("E121", "release has no signature", nil)
	}
	if !cleanField(slug) || !cleanField(version) || !isLowerHex64(sha256hex) {
		return fail("E121", "release identity is malformed (slug, version or sha256)", nil)
	}
	if rel.Alg != Alg {
		return fail("E121", fmt.Sprintf("release signature algorithm %q is not %s", clip(rel.Alg), Alg), nil)
	}
	sigBytes, err := signing.DecodeSig(rel.Sig)
	if err != nil {
		return fail("E121", "release signature is not valid base64", err)
	}
	v, err := verifierFor(tier)
	if err != nil {
		return fail("E121", "trusted key table is unusable", err)
	}
	err = v.Verify(SigningInput(slug, version, sha256hex), signing.Signature{KeyID: rel.KeyID, Sig: sigBytes})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, signing.ErrRevoked):
		return fail("E123", fmt.Sprintf("release key %q is revoked", clip(rel.KeyID)), err)
	case errors.Is(err, signing.ErrUnknownKey), errors.Is(err, signing.ErrWrongScope),
		errors.Is(err, signing.ErrWrongPurpose), errors.Is(err, signing.ErrExpired), errors.Is(err, signing.ErrNotYetValid):
		return fail("E122", fmt.Sprintf("release key %q is not trusted for the %s tier", clip(rel.KeyID), tier), err)
	default:
		return fail("E121", fmt.Sprintf("release signature by key %q does not verify", clip(rel.KeyID)), err)
	}
}

// fail builds the coded error; the underlying signing error rides along for errors.Is.
func fail(code, what string, cause error) error {
	if cause != nil {
		return errs.Wrap(code, what, cause)
	}
	return errs.New(code, what)
}

// clip bounds attacker-controlled text echoed in messages.
func clip(s string) string {
	if len(s) > 64 {
		return s[:64] + "..."
	}
	return s
}
