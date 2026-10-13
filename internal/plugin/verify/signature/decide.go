package signature

// decide.go — what an install does with a (possibly absent) release signature.
//
// Purpose: turn "this entry has / lacks a release_signature" into verify, warn or refuse, in one pure place the installer and the tests share.
// Inputs: Input (tier, slug, version, sha256, the entry's signature, the compat mode and the strict switch).
// Outputs: Outcome (Verified, or a Warning to print once) or a coded error.
// Constraints: no fail-open on a present, verifiable signature that is invalid, in any mode. Only a missing signature ever warns, and only in v1.4 mode. A tier with no pinned key cannot verify anything, so a signature on it counts as missing (final gate G1); a tier with a pinned key never falls back.

import (
	"fmt"

	"github.com/nself-org/cli/internal/errs"
)

// Input is one install decision.
type Input struct {
	Tier, Slug, Version, SHA256 string
	Release                     *Release // nil when the entry has none
	V15                         bool     // compat.V15() at the call site
	Strict                      bool     // refuse a missing signature even in v1.4 mode (NSELF_PLUGIN_REQUIRE_CHECKSUM on a stable plugin)
	AllowUnsigned               bool     // --allow-unsigned; the caller passes true only for local path or link installs
}

// Outcome reports a passing decision.
type Outcome struct {
	Verified bool   // a signature was checked and is good
	Warning  string // non-empty: print once and proceed
}

// Decide applies the rules above.
func Decide(in Input) (Outcome, error) {
	if err := CheckKeys(); err != nil {
		return Outcome{}, fail("E121", "trusted key table is unusable", err)
	}
	if in.Release != nil && Pinned(in.Tier) {
		if err := Verify(in.Tier, in.Slug, in.Version, in.SHA256, in.Release); err != nil {
			return Outcome{}, err
		}
		return Outcome{Verified: true}, nil
	}
	reason := "has no release signature"
	if in.Release != nil {
		reason = fmt.Sprintf("carries a release signature, but the %s tier has no trusted key in this CLI", in.Tier)
	}
	if in.AllowUnsigned {
		return Outcome{Warning: fmt.Sprintf("installing unsigned plugin %q %s (--allow-unsigned, local install)", in.Slug, in.Version)}, nil
	}
	if FloorRequires(in.Tier, in.Slug, in.Version) {
		return Outcome{}, errs.Newf("E107", "plugin %q %s %s, and every release since signing began must be signed", in.Slug, in.Version, reason)
	}
	if in.V15 || in.Strict {
		return Outcome{}, errs.Newf("E107", "plugin %q %s %s", in.Slug, in.Version, reason)
	}
	return Outcome{Warning: fmt.Sprintf("plugin %q %s %s; nself 1.5 refuses unsigned plugins", in.Slug, in.Version, reason)}, nil
}
