package plugin

// Purpose: carry the caller's explicit development signature-bypass consent.
// Inputs: install context, --force acknowledgement and legacy bypass env vars.
// Outputs: a scoped acknowledgement and the signature bypass decision.
// Constraints: never mutate process env; prod/staging always refuse bypasses.

import (
	"context"
	"fmt"
	"os"
)

type signatureBypassAcknowledgedKey struct{}

// WithSignatureBypassAcknowledged records --force for this install and its
// dependencies. Consent alone never requests a bypass: SKIP_VERIFY must be set.
func WithSignatureBypassAcknowledged(ctx context.Context, acknowledged bool) context.Context {
	return context.WithValue(ctx, signatureBypassAcknowledgedKey{}, acknowledged)
}

// checkSigBypassAllowedWithContext preserves legacy env callers while accepting
// command-scoped consent. A present signature on a pinned tier is still verified.
func checkSigBypassAllowedWithContext(ctx context.Context, env, _ string) (bool, error) {
	skipVerify := os.Getenv("NSELF_LICENSE_SKIP_VERIFY") == "1"
	acknowledged, _ := ctx.Value(signatureBypassAcknowledgedKey{}).(bool)
	forceSkip := os.Getenv("NSELF_LICENSE_SKIP_VERIFY_FORCE") == "1" || (skipVerify && acknowledged)
	if !skipVerify && !forceSkip {
		return false, nil
	}
	if env == "prod" || env == "staging" {
		return false, fmt.Errorf(
			"SECURITY: NSELF_LICENSE_SKIP_VERIFY and NSELF_LICENSE_SKIP_VERIFY_FORCE are not permitted in %s — remove these env vars",
			env,
		)
	}
	if !skipVerify || !forceSkip {
		return false, fmt.Errorf("NSELF_LICENSE_SKIP_VERIFY requires --force or NSELF_LICENSE_SKIP_VERIFY_FORCE=1; standalone skip is not permitted")
	}
	return true, nil
}
