package commands

// ssl_add_test.go — Unit tests for `nself ssl add` (ssl_add.go's runSSLAdd),
// which had zero direct test coverage before this ticket even though its
// path-construction helper (domainToFilesafe) and the layout it must agree
// with were already covered in ssl_install_test.go.
// P6-E11-W2-S3-T18: security command test floor.
//
// Security/correctness properties under test: runSSLAdd must refuse to run
// (and never touch certbot, docker, or the filesystem beyond MkdirAll) when
// its preconditions are not met — no project, no certbot on PATH, no
// ADMIN_EMAIL configured. Skipping any of these guards would let the
// command attempt certificate issuance with an invalid or missing identity,
// or silently no-op while reporting a misleading error.
// Inputs: temp project roots, a manipulated PATH, a stub `certbot` binary.
// Outputs: descriptive errors on each precondition failure.
// Constraints: no real certbot, docker, or network calls are made — every
// case here returns before runSSLAdd reaches exec.Command("certbot", ...).

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestRunSSLAdd_NoProject_Errors verifies runSSLAdd refuses to proceed
// outside an nself project rather than attempting certificate issuance
// against an undefined workdir.
func TestRunSSLAdd_NoProject_Errors(t *testing.T) {
	withProjectRoot(t, func(root string) {
		_ = root // deliberately no .env marker written
		err := runSSLAdd(sslAddCmd, []string{"custom.example.com"})
		if err == nil {
			t.Fatal("expected error outside an nself project, got nil")
		}
		if !strings.Contains(err.Error(), "no nself project found") {
			t.Errorf("error = %q, want it to mention 'no nself project found'", err.Error())
		}
	})
}

// TestRunSSLAdd_NoCertbot_Errors verifies runSSLAdd stops before ever
// checking ADMIN_EMAIL or writing to disk when certbot is not installed —
// a project misconfigured in ANY way should fail on the most actionable
// error, and "install certbot" must not be masked by a later check.
func TestRunSSLAdd_NoCertbot_Errors(t *testing.T) {
	compattest.Set(t, false) // D-0275: asserts v1.4 behaviour; v1.5 mode is not asserted by this test
	withProjectRoot(t, func(root string) {
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte("ADMIN_EMAIL=ops@example.com\n"), 0o644); err != nil {
			t.Fatalf("write .env: %v", err)
		}

		emptyBin := t.TempDir()
		t.Setenv("PATH", emptyBin)

		err := runSSLAdd(sslAddCmd, []string{"custom.example.com"})
		if err == nil {
			t.Fatal("expected error when certbot is not on PATH, got nil")
		}
		if !strings.Contains(err.Error(), "certbot") {
			t.Errorf("error = %q, want it to mention certbot", err.Error())
		}

		// The cert directory must NOT have been created — the command must
		// fail before any filesystem side effect beyond config loading.
		certDir := filepath.Join(root, "ssl", "certificates", domainToFilesafe("custom.example.com"))
		if _, statErr := os.Stat(certDir); statErr == nil {
			t.Error("cert directory was created despite the missing-certbot guard failing first")
		}
	})
}

// stubBinDir creates a temp directory containing an executable named `name`
// that does nothing and exits 0, and returns the directory. Used to satisfy
// exec.LookPath checks without invoking a real external tool.
//
// exec.LookPath only resolves a bare name against PATHEXT on Windows (.exe,
// .bat, .cmd, ...); a same-named file with no such extension is invisible to
// it even though it sits on PATH. Give the stub a `.exe` suffix on Windows so
// the lookup succeeds there the same way it does on Unix — this only needs
// to be *found*, never actually executed, since every caller fails before
// reaching exec.Command on the resolved path.
func stubBinDir(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", name, err)
	}
	return dir
}

// TestRunSSLAdd_MissingAdminEmail_Errors verifies runSSLAdd refuses to
// provision a certificate when no ADMIN_EMAIL is configured — certbot
// registration requires a real contact identity, and proceeding without
// one would either fail deep inside certbot with a confusing error or
// register the account under no identity at all.
func TestRunSSLAdd_MissingAdminEmail_Errors(t *testing.T) {
	compattest.Set(t, false) // D-0275: asserts v1.4 behaviour; v1.5 mode is not asserted by this test
	withProjectRoot(t, func(root string) {
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte("BASE_DOMAIN=example.com\n"), 0o644); err != nil {
			t.Fatalf("write .env: %v", err)
		}
		t.Setenv("ADMIN_EMAIL", "") // config.Load reads ADMIN_EMAIL from process env

		t.Setenv("PATH", stubBinDir(t, "certbot"))

		err := runSSLAdd(sslAddCmd, []string{"custom.example.com"})
		if err == nil {
			t.Fatal("expected error when ADMIN_EMAIL is unset, got nil")
		}
		if !strings.Contains(err.Error(), "ADMIN_EMAIL") {
			t.Errorf("error = %q, want it to mention ADMIN_EMAIL", err.Error())
		}

		certDir := filepath.Join(root, "ssl", "certificates", domainToFilesafe("custom.example.com"))
		if _, statErr := os.Stat(certDir); statErr == nil {
			t.Error("cert directory was created despite the missing-ADMIN_EMAIL guard failing first")
		}
	})
}
