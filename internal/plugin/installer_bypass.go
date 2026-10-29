package plugin

// Purpose: carry the caller's explicit --force acknowledgement of
// NSELF_LICENSE_SKIP_VERIFY and gate that licence-check skip.
// Inputs: install context, the --force acknowledgement and the
// NSELF_LICENSE_SKIP_VERIFY env var (a project env file cannot set it:
// config.IsProtectedEnvKey).
// Outputs: a scoped acknowledgement and the gate decision.
// Constraints: never mutate process env; prod/staging always refuse; the
// legacy NSELF_LICENSE_SKIP_VERIFY_FORCE env var no longer stands in for
// --force. An allowed skip never skips a registry signature or checksum
// check: verifyReleaseSignature has no bypass (P7-PLUG-87 review H1).

import (
	"context"
	"fmt"
	"os"
)

type signatureBypassAcknowledgedKey struct{}

// WithSignatureBypassAcknowledged records --force for this install and its
// dependencies. Consent alone never requests a skip: SKIP_VERIFY must be set.
func WithSignatureBypassAcknowledged(ctx context.Context, acknowledged bool) context.Context {
	return context.WithValue(ctx, signatureBypassAcknowledgedKey{}, acknowledged)
}

// checkSigBypassAllowedWithContext reports whether NSELF_LICENSE_SKIP_VERIFY
// is set and acknowledged by --force on this command line. Env alone is never
// enough. It refuses in prod/staging.
func checkSigBypassAllowedWithContext(ctx context.Context, env, _ string) (bool, error) {
	if os.Getenv("NSELF_LICENSE_SKIP_VERIFY") != "1" {
		return false, nil
	}
	if env == "prod" || env == "staging" {
		return false, fmt.Errorf(
			"SECURITY: NSELF_LICENSE_SKIP_VERIFY is not permitted in %s — remove it from the environment",
			env,
		)
	}
	if acknowledged, _ := ctx.Value(signatureBypassAcknowledgedKey{}).(bool); !acknowledged {
		return false, fmt.Errorf("NSELF_LICENSE_SKIP_VERIFY requires the --force flag on the command line; the environment alone (NSELF_LICENSE_SKIP_VERIFY_FORCE included) is not accepted")
	}
	return true, nil
}
