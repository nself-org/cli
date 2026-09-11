package plugin

// Purpose: Plugin security checks — checksum/signature verification, Ed25519
//          CRL author revocation, license validation, EOL blocking, and the
//          anonymous install-event telemetry POST.
// Inputs:  archive file paths, hex-encoded keys/sigs, context with timeout.
// Outputs: error on any policy violation; nil on pass or when non-fatal
//          (network offline, non-stable status, or author not found in CRL).
// Constraints: A present-but-wrong checksum/signature always refuses the
//              install, for every publishStatus. A MISSING checksum/signature
//              on an effectively-stable plugin (EffectiveStatus == "stable",
//              which includes an absent status) only refuses when
//              NSELF_PLUGIN_REQUIRE_CHECKSUM=1 is set — default is warn and
//              proceed (see pluginRequireChecksumEnv doc comment; FIX-CLI-6).
//              License check tries all keys; first valid entitlement match wins.
//              CRL fetch errors are non-fatal — warns to stderr, never blocks.
//              postInstallEvent always runs in a goroutine; errors are silent.
// SPORT: security/verification pipeline; callers: installLocked in installer.go

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
)

// postInstallEvent POSTs an anonymous install count event to the registry worker.
// It is always called in a goroutine and swallows all errors silently.
// The instanceId is a SHA-256 hex hash of the machineID — opaque, no PII.
func postInstallEvent(pluginName string) {
	mid := machineID() // 16-char hex
	// SHA-256 of the machineID to produce the required 64-char hex instanceId
	h := sha256.Sum256([]byte(mid))
	instanceID := hex.EncodeToString(h[:])

	body := `{"instanceId":"` + instanceID + `"}`
	url := "https://plugins.nself.org/plugins/" + pluginName + "/install-event"

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return // silent
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httptimeout.Plugin.Do(req)
	if err != nil {
		return // silent
	}
	_ = resp.Body.Close()
}

// pluginRequireChecksumEnv opts a machine into hard-refusing installs of any
// effectively-stable plugin (EffectiveStatus == "stable", which includes an
// absent status) that is missing a checksum or signature, instead of the
// default warn-and-proceed. Default off: as of 2026-09-04 the live registry
// carries a checksum on only 47 of 177 plugins, so refusing by default would
// refuse the other 130 installs outright (FIX-CLI-6, plugins#83 follow-up).
// Flip this default once registry coverage reaches 177/177.
const pluginRequireChecksumEnv = "NSELF_PLUGIN_REQUIRE_CHECKSUM"

// pluginChecksumRequired reports whether the hard-refusal mode above is on.
func pluginChecksumRequired() bool {
	return os.Getenv(pluginRequireChecksumEnv) == "1"
}

// verifyChecksum computes the SHA256 hash of the file at filePath and compares
// it to expectedHash (hex-encoded). Returns an error if the hashes differ —
// this mismatch check is unconditional: it applies to every publishStatus
// (including "beta"/"deprecated"), since a checksum that IS present and
// wrong is never acceptable regardless of lifecycle stage.
//
// publishStatus is the plugin's raw lifecycle status from the registry
// ("stable", "beta", "alpha", "experimental", "" for absent, etc.) — compared
// via EffectiveStatus so an absent status is treated exactly like an explicit
// "stable" one. When expectedHash is empty (no checksum in the registry) the
// default behavior is to warn and proceed for every status, including
// effectively-stable ones — see pluginRequireChecksumEnv. Only when that env
// var opts in does a missing checksum on an effectively-stable plugin return
// errs.ErrPluginMissingChecksum and refuse the install.
func verifyChecksum(filePath string, expectedHash string, publishStatus string) error {
	if expectedHash == "" {
		if pluginChecksumRequired() && EffectiveStatus(publishStatus) == "stable" {
			return fmt.Errorf("plugin %q is missing required checksum for stable publishStatus — install refused: %w",
				filePath, errs.ErrPluginMissingChecksum)
		}
		return nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file for checksum: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("computing checksum: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expectedHash) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actual)
	}
	return nil
}

// resolveArtifactChecksum picks the registry checksum that matches the
// artifact downloadPluginPackageForTier actually fetched, identified by
// artifactKind (ArtifactKindSource, or a platform string from PlatformArch()).
//
// The source tarball and each platform's binary tarball are different bytes
// for the same plugin+version, so a source checksum can never validate a
// platform download or vice versa — comparing the wrong one is not a
// stricter check, it is a guaranteed-wrong one (plugins#83, the bug this
// function exists to close).
//
// For the source artifact this returns manifest.Checksum verbatim, including
// when it is empty — verifyChecksum's own warn-and-proceed leniency for a
// missing SOURCE checksum (FIX-CLI-6, a documented and tracked registry
// coverage gap) still applies, unchanged, to whatever this returns.
//
// For a platform artifact, a missing checksum in
// manifest.PlatformChecksums[artifactKind] is refused HERE, unconditionally
// — it never reaches verifyChecksum's lenient empty-string path, and no env
// var (NSELF_PLUGIN_REQUIRE_CHECKSUM included) changes that. The FIX-CLI-6
// leniency exists for the source-checksum coverage gap; it was never a
// license to install an actual downloaded EXECUTABLE with zero
// verification. A release that predates PlatformChecksums, or one platform's
// checksum that was never backfilled, are both registry data gaps — the fix
// is a registry checksum, not a flag that makes the install proceed anyway.
func resolveArtifactChecksum(manifest PluginManifest, artifactKind string) (string, error) {
	if artifactKind == ArtifactKindSource || artifactKind == "" {
		return manifest.Checksum, nil
	}

	checksum, ok := manifest.PlatformChecksums[artifactKind]
	if !ok || checksum == "" {
		return "", fmt.Errorf(
			"plugin %q: registry has no checksum for platform %q at version %s — refusing to install an unverified binary (report to nself-org/plugins)",
			manifest.Name, artifactKind, manifest.Version)
	}
	return checksum, nil
}

// verifyPluginSignature verifies that the Ed25519 signature stored in the
// plugin's registry manifest matches the SHA-256 hash of the downloaded
// tarball. The public key is pinned in the registry (never fetched at verify
// time, preventing TOCTOU attacks).
//
// publishStatus is the plugin's raw lifecycle status from the registry
// ("stable", "beta", "alpha", "experimental", "" for absent, etc.) —
// compared via EffectiveStatus so an absent status is treated exactly like
// an explicit "stable" one. When authorPublicKeyHex or signatureHex is empty
// (no signature in the registry) the default behavior is to skip
// verification with a warning for every status, including
// effectively-stable ones — see pluginRequireChecksumEnv, which also governs
// this gate. Only when that env var opts in does a missing signature on an
// effectively-stable plugin return errs.ErrPluginUnsigned and refuse the
// install. A signature that IS present and does not verify always refuses,
// regardless of status — see the ed25519.Verify check below.
func verifyPluginSignature(archivePath, authorPublicKeyHex, signatureHex, publishStatus string) error {
	if authorPublicKeyHex == "" || signatureHex == "" {
		if pluginChecksumRequired() && EffectiveStatus(publishStatus) == "stable" {
			return fmt.Errorf("plugin is missing required signature for stable publishStatus — install refused: %w",
				errs.ErrPluginUnsigned)
		}
		return nil
	}

	pkBytes, err := hex.DecodeString(authorPublicKeyHex)
	if err != nil {
		return fmt.Errorf("decoding author public key: %w", err)
	}
	if len(pkBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("author public key has wrong length: expected %d bytes, got %d", ed25519.PublicKeySize, len(pkBytes))
	}
	pubKey := ed25519.PublicKey(pkBytes)

	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("decoding plugin signature: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("plugin signature has wrong length: expected %d bytes, got %d", ed25519.SignatureSize, len(sigBytes))
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive for signature verification: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing archive for signature verification: %w", err)
	}
	digest := h.Sum(nil)

	if !ed25519.Verify(pubKey, digest, sigBytes) {
		return fmt.Errorf("plugin signature verification failed: tarball does not match registry signature (possible tampering)")
	}
	return nil
}
