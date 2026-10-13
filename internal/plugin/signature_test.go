package plugin

// signature_test.go — checksum verification tests (verifyChecksum, E124).
// The release-signature tests live in installer_signature_test.go and
// verify/signature; the old author-key verifyPluginSignature tests were
// removed with the function (P7-PLUG-23).

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// writeTempFile writes content to a temp file and returns its path.
func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "archive-*.bin")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return f.Name()
}

// TestVerifyChecksum_MissingChecksumDefaultWarnOnly verifies that, by
// default (NSELF_PLUGIN_REQUIRE_CHECKSUM unset), a missing checksum is
// permitted — never refused — for every status, including an explicit
// "stable" and an absent ("") one. FIX-CLI-6: registry coverage is 47/177 as
// of 2026-09-04, so refusing by default would refuse the other 130 installs.
func TestVerifyChecksum_MissingChecksumDefaultWarnOnly(t *testing.T) {
	content := []byte("plugin archive content")
	archivePath := writeTempFile(t, content)

	for _, status := range []string{"stable", "", "alpha", "beta", "deprecated", "experimental"} {
		t.Run("status="+status, func(t *testing.T) {
			if err := verifyChecksum(archivePath, "", status); err != nil {
				t.Errorf("default mode: missing checksum should warn and proceed (nil), got: %v", err)
			}
		})
	}
}

// TestVerifyChecksum_MissingChecksumHardModeRefusesEffectivelyStable verifies
// that with NSELF_PLUGIN_REQUIRE_CHECKSUM=1 set, a missing checksum is
// refused with ErrPluginMissingChecksum for both an explicit "stable" status
// and an absent ("") one (EffectiveStatus treats them the same — this is the
// actual FIX-CLI-6 defect: before this fix "" never hit the hard-fail branch
// even in hard mode).
func TestVerifyChecksum_MissingChecksumHardModeRefusesEffectivelyStable(t *testing.T) {
	t.Setenv(pluginRequireChecksumEnv, "1")
	content := []byte("plugin archive content")
	archivePath := writeTempFile(t, content)

	for _, status := range []string{"stable", ""} {
		t.Run("status="+status, func(t *testing.T) {
			err := verifyChecksum(archivePath, "", status)
			if err == nil {
				t.Fatal("hard mode: effectively-stable plugin with empty checksum should return ErrPluginMissingChecksum, got nil")
			}
			if !errors.Is(err, errs.ErrPluginMissingChecksum) {
				t.Errorf("expected ErrPluginMissingChecksum, got: %v", err)
			}
		})
	}
}

// TestVerifyChecksum_MissingChecksumHardModeNonStableSkip verifies that,
// even in hard mode, beta/deprecated plugins with a missing checksum still
// skip verification — the hard-mode requirement only applies to
// EffectiveStatus == "stable".
func TestVerifyChecksum_MissingChecksumHardModeNonStableSkip(t *testing.T) {
	t.Setenv(pluginRequireChecksumEnv, "1")
	content := []byte("plugin archive content")
	archivePath := writeTempFile(t, content)

	for _, status := range []string{"alpha", "beta", "deprecated", "experimental"} {
		t.Run("status="+status, func(t *testing.T) {
			if err := verifyChecksum(archivePath, "", status); err != nil {
				t.Errorf("non-stable plugin with empty checksum should return nil even in hard mode, got: %v", err)
			}
		})
	}
}

// TestVerifyChecksum_ValidChecksum verifies correct checksum passes.
func TestVerifyChecksum_ValidChecksum(t *testing.T) {
	content := []byte("plugin archive content for checksum test")
	archivePath := writeTempFile(t, content)

	h := sha256.New()
	h.Write(content)
	expectedHex := hex.EncodeToString(h.Sum(nil))

	if err := verifyChecksum(archivePath, expectedHex, "stable"); err != nil {
		t.Fatalf("valid checksum should pass: %v", err)
	}
}

// TestVerifyChecksum_MismatchAlwaysRefusesRegardlessOfStatus verifies that a
// PRESENT-but-wrong checksum always refuses the install, for every
// publishStatus (including "beta"/"deprecated") — a present-but-wrong
// checksum is never acceptable regardless of lifecycle stage, and this check
// does not depend on NSELF_PLUGIN_REQUIRE_CHECKSUM.
func TestVerifyChecksum_MismatchAlwaysRefusesRegardlessOfStatus(t *testing.T) {
	content := []byte("plugin archive content for mismatch test")
	archivePath := writeTempFile(t, content)
	wrongHex := hex.EncodeToString(make([]byte, sha256.Size)) // all-zero, guaranteed wrong

	for _, status := range []string{"", "stable", "beta", "deprecated"} {
		t.Run("status="+status, func(t *testing.T) {
			err := verifyChecksum(archivePath, wrongHex, status)
			if err == nil {
				t.Fatalf("present-but-wrong checksum must refuse for status %q, got nil", status)
			}
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E124" {
				t.Fatalf("mismatch must be E124, got %v", err)
			}
		})
	}
}
