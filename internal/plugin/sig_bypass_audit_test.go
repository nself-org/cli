package plugin

// Purpose: Unit tests for plugin signature verification bypass controls
//          (T10 — MEDIUM security finding: prod/staging block + dev audit log).
// Inputs:  Environment variables NSELF_LICENSE_SKIP_VERIFY,
//          NSELF_LICENSE_SKIP_VERIFY_FORCE; env string passed to helper.
// Outputs: Confirms checkSigBypassAllowed returns SECURITY error in prod/staging
//          and refuses both vars in dev without --force (env alone never
//          acknowledges the skip; P7-PLUG-87 review H1).
// SPORT: security-audit; ticket P2-E2-W2-S3-T10

import (
	"strings"
	"testing"
)

// TestCheckSigBypassAllowed_ProdBlocked verifies that prod env with both bypass
// vars set returns a SECURITY error and bypassed=false.
func TestCheckSigBypassAllowed_ProdBlocked(t *testing.T) {
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "1")

	bypassed, err := checkSigBypassAllowed("prod", "test-plugin")
	if err == nil {
		t.Fatal("expected SECURITY error for prod bypass, got nil")
	}
	if bypassed {
		t.Fatal("expected bypassed=false when prod env rejects bypass")
	}
	if !strings.HasPrefix(err.Error(), "SECURITY:") {
		t.Fatalf("expected error starting with 'SECURITY:', got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "prod") {
		t.Fatalf("expected error to mention env 'prod', got: %q", err.Error())
	}
}

// TestCheckSigBypassAllowed_StagingBlocked verifies that staging env also
// triggers the production-grade SECURITY error.
func TestCheckSigBypassAllowed_StagingBlocked(t *testing.T) {
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "1")

	bypassed, err := checkSigBypassAllowed("staging", "test-plugin")
	if err == nil {
		t.Fatal("expected SECURITY error for staging bypass, got nil")
	}
	if bypassed {
		t.Fatal("expected bypassed=false when staging env rejects bypass")
	}
	if !strings.HasPrefix(err.Error(), "SECURITY:") {
		t.Fatalf("expected error starting with 'SECURITY:', got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Fatalf("expected error to mention env 'staging', got: %q", err.Error())
	}
}

// TestCheckSigBypassAllowed_ForceEnvAloneRefused verifies that dev env with
// both vars set but no --force acknowledgement is refused: the legacy FORCE
// env var no longer stands in for the command-line flag.
func TestCheckSigBypassAllowed_ForceEnvAloneRefused(t *testing.T) {
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "1")

	bypassed, err := checkSigBypassAllowed("dev", "test-plugin")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected a --force refusal for env-only bypass, got: %v", err)
	}
	if bypassed {
		t.Fatal("expected bypassed=false without --force")
	}
}

// TestCheckSigBypassAllowed_StandaloneSkipVerifyRejected verifies that setting
// only NSELF_LICENSE_SKIP_VERIFY without NSELF_LICENSE_SKIP_VERIFY_FORCE is
// rejected even in dev.
func TestCheckSigBypassAllowed_StandaloneSkipVerifyRejected(t *testing.T) {
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "")

	bypassed, err := checkSigBypassAllowed("dev", "test-plugin")
	if err == nil {
		t.Fatal("expected error for standalone NSELF_LICENSE_SKIP_VERIFY, got nil")
	}
	if bypassed {
		t.Fatal("expected bypassed=false for standalone skip")
	}
}

// TestCheckSigBypassAllowed_NoVars verifies that with no bypass vars set,
// the normal verification path is returned (bypassed=false, err=nil).
func TestCheckSigBypassAllowed_NoVars(t *testing.T) {
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "")

	bypassed, err := checkSigBypassAllowed("prod", "test-plugin")
	if err != nil {
		t.Fatalf("expected nil error with no bypass vars, got: %v", err)
	}
	if bypassed {
		t.Fatal("expected bypassed=false with no bypass vars")
	}
}
