package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// TestResolveArtifactChecksum_SourceUsesManifestChecksum verifies that for
// the source artifact (ArtifactKindSource, and the zero value "" — a
// download that never got as far as reporting a kind), resolveArtifactChecksum
// returns manifest.Checksum verbatim, unaffected by whatever is in
// PlatformChecksums.
func TestResolveArtifactChecksum_SourceUsesManifestChecksum(t *testing.T) {
	manifest := PluginManifest{
		Name:              "example",
		Version:           "1.0.1",
		Checksum:          "deadbeef",
		PlatformChecksums: map[string]string{"linux-amd64": "should-not-be-used"},
	}

	for _, kind := range []string{ArtifactKindSource, ""} {
		t.Run("kind="+kind, func(t *testing.T) {
			got, err := resolveArtifactChecksum(manifest, kind)
			if err != nil {
				t.Fatalf("resolveArtifactChecksum: %v", err)
			}
			if got != "deadbeef" {
				t.Errorf("got %q, want manifest.Checksum %q", got, manifest.Checksum)
			}
		})
	}
}

// TestResolveArtifactChecksum_SourceEmptyChecksumPassesThrough verifies that
// an empty manifest.Checksum for the source artifact comes back as "", not
// an error — the FIX-CLI-6 warn-and-proceed leniency for a missing SOURCE
// checksum is verifyChecksum's decision to make, not this function's.
func TestResolveArtifactChecksum_SourceEmptyChecksumPassesThrough(t *testing.T) {
	manifest := PluginManifest{Name: "example", Version: "1.0.1"}

	got, err := resolveArtifactChecksum(manifest, ArtifactKindSource)
	if err != nil {
		t.Fatalf("resolveArtifactChecksum: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// TestResolveArtifactChecksum_PlatformUsesMatchingEntry verifies that a
// platform artifact resolves to PlatformChecksums[platform], the entry
// matching the artifact actually downloaded — not manifest.Checksum (the
// SOURCE tarball's checksum, different bytes entirely) and not a different
// platform's entry.
func TestResolveArtifactChecksum_PlatformUsesMatchingEntry(t *testing.T) {
	manifest := PluginManifest{
		Name:     "example",
		Version:  "1.0.1",
		Checksum: "source-checksum-must-not-be-used",
		PlatformChecksums: map[string]string{
			"darwin-arm64": "darwin-arm64-checksum",
			"linux-amd64":  "linux-amd64-checksum",
		},
	}

	got, err := resolveArtifactChecksum(manifest, "linux-amd64")
	if err != nil {
		t.Fatalf("resolveArtifactChecksum: %v", err)
	}
	if got != "linux-amd64-checksum" {
		t.Errorf("got %q, want the linux-amd64 entry, not the source checksum or a different platform's", got)
	}
}

// TestResolveArtifactChecksum_PlatformMissingEntryRefusesUnconditionally
// verifies the absent-checksum policy (plugins#83 item 8): a platform
// artifact with NO matching registry entry is refused with an error —
// regardless of NSELF_PLUGIN_REQUIRE_CHECKSUM, regardless of whether
// manifest.Checksum (the source checksum) happens to be present, and
// regardless of whether PlatformChecksums is nil or just missing this one
// platform's key. This is what makes the policy impossible to bypass: there
// is no code path, env var, or registry shape that turns a missing platform
// checksum into an empty string reaching verifyChecksum's lenient branch.
func TestResolveArtifactChecksum_PlatformMissingEntryRefusesUnconditionally(t *testing.T) {
	cases := []struct {
		name              string
		platformChecksums map[string]string
		requireChecksum   bool
	}{
		{name: "nil map, default mode", platformChecksums: nil, requireChecksum: false},
		{name: "nil map, hard mode", platformChecksums: nil, requireChecksum: true},
		{name: "other platforms present, default mode", platformChecksums: map[string]string{"darwin-arm64": "abc123"}, requireChecksum: false},
		{name: "other platforms present, hard mode", platformChecksums: map[string]string{"darwin-arm64": "abc123"}, requireChecksum: true},
		{name: "empty-string entry for this platform", platformChecksums: map[string]string{"linux-amd64": ""}, requireChecksum: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.requireChecksum {
				t.Setenv(pluginRequireChecksumEnv, "1")
			}
			manifest := PluginManifest{
				Name:              "example",
				Version:           "1.0.1",
				Checksum:          "source-checksum-present-but-irrelevant",
				PlatformChecksums: tc.platformChecksums,
			}

			_, err := resolveArtifactChecksum(manifest, "linux-amd64")
			if err == nil {
				t.Fatal("expected an error refusing the install, got nil")
			}
			if !strings.Contains(err.Error(), "linux-amd64") {
				t.Errorf("error should name the platform that has no checksum: %v", err)
			}
		})
	}
}

// TestPlatformArtifact_VerifiesAgainstMatchingPlatformChecksum is the
// end-to-end shape of Step 5 for a platform download: resolve the checksum
// for the artifact actually downloaded, then verify the archive bytes
// against it. A correct platform checksum passes.
func TestPlatformArtifact_VerifiesAgainstMatchingPlatformChecksum(t *testing.T) {
	content := []byte("fake linux-amd64 binary tarball bytes")
	archivePath := writeTempFile(t, content)
	sum := sha256.Sum256(content)
	platformChecksum := hex.EncodeToString(sum[:])

	manifest := PluginManifest{
		Name:     "example",
		Version:  "1.0.1",
		Checksum: "unrelated-source-checksum",
		PlatformChecksums: map[string]string{
			"linux-amd64": platformChecksum,
		},
	}

	expected, err := resolveArtifactChecksum(manifest, "linux-amd64")
	if err != nil {
		t.Fatalf("resolveArtifactChecksum: %v", err)
	}
	if err := verifyChecksum(archivePath, expected, "stable"); err != nil {
		t.Fatalf("correct platform checksum should verify: %v", err)
	}
}

// TestPlatformArtifact_MismatchRejected verifies that a platform download
// whose bytes do not match its recorded platform checksum is rejected — the
// same as any other present-but-wrong checksum (mirrors
// TestVerifyChecksum_MismatchAlwaysRefusesRegardlessOfStatus, for the
// platform-checksum path specifically).
func TestPlatformArtifact_MismatchRejected(t *testing.T) {
	content := []byte("fake linux-amd64 binary tarball bytes")
	archivePath := writeTempFile(t, content)
	wrongChecksum := hex.EncodeToString(make([]byte, sha256.Size)) // all-zero, guaranteed wrong

	manifest := PluginManifest{
		Name:    "example",
		Version: "1.0.1",
		PlatformChecksums: map[string]string{
			"linux-amd64": wrongChecksum,
		},
	}

	expected, err := resolveArtifactChecksum(manifest, "linux-amd64")
	if err != nil {
		t.Fatalf("resolveArtifactChecksum: %v", err)
	}
	if err := verifyChecksum(archivePath, expected, "stable"); err == nil {
		t.Fatal("tampered/mismatched platform artifact should fail verification, got nil error")
	}
}

// TestPlatformArtifact_AbsentChecksumNeverReachesLenientPath verifies the
// absent-checksum policy end to end: a platform artifact with no registry
// checksum never reaches verifyChecksum's warn-and-proceed branch at all —
// resolveArtifactChecksum refuses first, before any checksum comparison
// (lenient or not) would run.
func TestPlatformArtifact_AbsentChecksumNeverReachesLenientPath(t *testing.T) {
	content := []byte("fake linux-amd64 binary tarball bytes")
	archivePath := writeTempFile(t, content)
	_ = archivePath // the archive is never reached; resolveArtifactChecksum fails first

	manifest := PluginManifest{
		Name:              "example",
		Version:           "1.0.1",
		PlatformChecksums: nil,
	}

	if _, err := resolveArtifactChecksum(manifest, "linux-amd64"); err == nil {
		t.Fatal("expected resolveArtifactChecksum to refuse before any verification would run")
	}
}
