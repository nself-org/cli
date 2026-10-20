package signature

// verify.go — checks one registry release_signature against the compiled-in keys.
//
// Purpose: verify the release_signature of a registry or catalog entry over the domain-separated string of hq PLUG D7, for one tier.
// Inputs: tier, slug, version, the lowercase hex sha256 of the signed tarball, and the entry's release_signature.
// Outputs: nil, or a *errs.CLIError: E121 bad or malformed signature, E122 unknown key (also a key of the other tier), E123 revoked key.
// Constraints: pure (no I/O, no clock). Fail closed: any problem, including a broken key table, is an error. The checksum compare (E124) happens before this, in the installer.

import (
	"encoding/hex"
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
	_, err := hex.DecodeString(s)
	return err == nil && len(s) == 64 && s == strings.ToLower(s)
}

// Verify checks rel for the given tier, slug, version and tarball sha256.
func Verify(tier, slug, version, sha256hex string, rel *Release) error {
	if !cleanField(slug) || !cleanField(version) || !isLowerHex64(sha256hex) {
		return verifyMessage(tier, nil, rel, true)
	}
	return verifyMessage(tier, SigningInput(slug, version, sha256hex), rel, false)
}

// verifyMessage is the check Verify and VerifyPlatform share, so the two
// domains cannot drift in their error codes. malformed means the identity
// fields could not be put in a signing input at all (msg is then nil).
func verifyMessage(tier string, msg []byte, rel *Release, malformed bool) error {
	if rel == nil {
		return fail("E121", "release has no signature", nil)
	}
	if malformed {
		return fail("E121", "release identity is malformed (slug, version, platform or sha256)", nil)
	}
	if rel.Alg != Alg {
		return fail("E121", fmt.Sprintf("release signature algorithm %.64q is not %s", rel.Alg, Alg), nil)
	}
	sigBytes, err := signing.DecodeSig(rel.Sig)
	if err != nil {
		return fail("E121", "release signature is not valid base64", err)
	}
	v, err := verifierFor(tier)
	if err != nil {
		return fail("E121", "trusted key table is unusable", err)
	}
	err = v.Verify(msg, signing.Signature{KeyID: rel.KeyID, Sig: sigBytes})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, signing.ErrRevoked):
		return fail("E123", fmt.Sprintf("release key %.64q is revoked", rel.KeyID), err)
	case errors.Is(err, signing.ErrUnknownKey), errors.Is(err, signing.ErrWrongScope),
		errors.Is(err, signing.ErrWrongPurpose), errors.Is(err, signing.ErrExpired), errors.Is(err, signing.ErrNotYetValid):
		return fail("E122", fmt.Sprintf("release key %.64q is not trusted for the %s tier", rel.KeyID, tier), err)
	default:
		return fail("E121", fmt.Sprintf("release signature by key %.64q does not verify", rel.KeyID), err)
	}
}

// fail builds the coded error; the underlying signing error (nil for none)
// rides along for errors.Is. Echoed key ids and algorithms are cut to 64
// characters because they come from the registry.
func fail(code, what string, cause error) error {
	return errs.Wrap(code, what, cause)
}
