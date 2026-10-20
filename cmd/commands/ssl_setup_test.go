package commands

// ssl_setup_test.go — Unit tests for `nself ssl setup` (ssl_setup.go's
// runSSLSetup), which had zero direct test coverage before this ticket.
// P6-E11-W2-S3-T18: security command test floor.
//
// Security/correctness properties under test: runSSLSetup gates on several
// preconditions before ever invoking certbot with real DNS-01 credentials —
// a real project, certbot present, a real (non-localhost) BASE_DOMAIN, a
// registration email, and a supported DNS provider name. Each gate must
// fail closed: skipping any one of them risks certbot running with an
// undefined domain/identity, or a provider name silently falling through to
// the wrong `--dns-*` credentials file.
// Inputs: temp project roots, a manipulated PATH, a stub `certbot` binary.
// Outputs: descriptive errors on each precondition failure.
// Constraints: no real certbot/docker/network calls — every case returns
// before runSSLSetup reaches exec.Command("certbot", ...).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// resetSSLSetupFlags restores sslSetupCmd's flags to their registered
// defaults so tests don't leak state into each other via the shared
// package-level *cobra.Command.
func resetSSLSetupFlags(t *testing.T) {
	t.Helper()
	_ = sslSetupCmd.Flags().Set("provider", "cloudflare")
	_ = sslSetupCmd.Flags().Set("wildcard", "false")
	_ = sslSetupCmd.Flags().Set("email", "")
	_ = sslSetupCmd.Flags().Set("staging", "false")
	_ = sslSetupCmd.Flags().Set("install-cron", "false")
}

func TestRunSSLSetup_NoProject_Errors(t *testing.T) {
	withProjectRoot(t, func(root string) {
		resetSSLSetupFlags(t)
		err := runSSLSetup(sslSetupCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "no nself project found") {
			t.Fatalf("expected 'no nself project found' error, got %v", err)
		}
	})
}

func TestRunSSLSetup_NoCertbot_Errors(t *testing.T) {
	compattest.Set(t, false) // D-0275: asserts v1.4 behaviour; v1.5 mode is not asserted by this test
	withProjectRoot(t, func(root string) {
		resetSSLSetupFlags(t)
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte("BASE_DOMAIN=example.com\nADMIN_EMAIL=ops@example.com\n"), 0o644); err != nil {
			t.Fatalf("write .env: %v", err)
		}
		t.Setenv("PATH", t.TempDir())

		err := runSSLSetup(sslSetupCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "certbot") {
			t.Fatalf("expected a certbot-not-found error, got %v", err)
		}
	})
}

// TestRunSSLSetup_LocalhostDomain_Rejected verifies the guard that stops
// SSL setup from ever running against BASE_DOMAIN=localhost — a real
// regression here would attempt (and fail deep inside certbot, or worse,
// hang on an ACME challenge) issuance for a domain that can never validate.
func TestRunSSLSetup_LocalhostDomain_Rejected(t *testing.T) {
	compattest.Set(t, false) // D-0275: asserts v1.4 behaviour; v1.5 mode is not asserted by this test
	withProjectRoot(t, func(root string) {
		resetSSLSetupFlags(t)
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte("BASE_DOMAIN=localhost\nADMIN_EMAIL=ops@example.com\n"), 0o644); err != nil {
			t.Fatalf("write .env: %v", err)
		}
		t.Setenv("PATH", stubBinDir(t, "certbot"))

		err := runSSLSetup(sslSetupCmd, nil)
		if err == nil {
			t.Fatal("expected error for BASE_DOMAIN=localhost, got nil")
		}
		if !strings.Contains(err.Error(), "BASE_DOMAIN") {
			t.Errorf("error = %q, want it to mention BASE_DOMAIN", err.Error())
		}
	})
}

// TestRunSSLSetup_MissingEmail_FlagAndEnvBothEmpty_Errors verifies both the
// --email flag AND ADMIN_EMAIL must be checked — a command that only
// checked one would silently register with an empty identity when the
// other source was used.
func TestRunSSLSetup_MissingEmail_FlagAndEnvBothEmpty_Errors(t *testing.T) {
	compattest.Set(t, false) // D-0275: asserts v1.4 behaviour; v1.5 mode is not asserted by this test
	withProjectRoot(t, func(root string) {
		resetSSLSetupFlags(t)
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte("BASE_DOMAIN=example.com\n"), 0o644); err != nil {
			t.Fatalf("write .env: %v", err)
		}
		t.Setenv("ADMIN_EMAIL", "")
		t.Setenv("PATH", stubBinDir(t, "certbot"))

		err := runSSLSetup(sslSetupCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "email") {
			t.Fatalf("expected an email-required error, got %v", err)
		}
	})
}

// TestRunSSLSetup_UnsupportedProvider_Rejected verifies an unrecognized
// --provider value is rejected rather than silently falling through with
// no DNS plugin flag — that would make certbot attempt HTTP-01 or fail
// deep inside the certbot invocation with a confusing error, instead of
// failing fast with a clear message naming the supported providers.
func TestRunSSLSetup_UnsupportedProvider_Rejected(t *testing.T) {
	compattest.Set(t, false) // D-0275: asserts v1.4 behaviour; v1.5 mode is not asserted by this test
	withProjectRoot(t, func(root string) {
		resetSSLSetupFlags(t)
		defer resetSSLSetupFlags(t)
		_ = sslSetupCmd.Flags().Set("provider", "not-a-real-provider")
		_ = sslSetupCmd.Flags().Set("email", "ops@example.com")

		if err := os.WriteFile(filepath.Join(root, ".env"), []byte("BASE_DOMAIN=example.com\n"), 0o644); err != nil {
			t.Fatalf("write .env: %v", err)
		}
		t.Setenv("PATH", stubBinDir(t, "certbot"))

		err := runSSLSetup(sslSetupCmd, nil)
		if err == nil {
			t.Fatal("expected error for an unsupported provider, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported SSL provider") {
			t.Errorf("error = %q, want it to mention 'unsupported SSL provider'", err.Error())
		}
	})
}
