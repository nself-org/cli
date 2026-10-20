package plugin

// installer_signature.go — Step 5b of the install sequence: release-signature verification.
//
// Purpose: verify the registry's release_signature (hq PLUG D7, contract plugins.release-signature v1) against the keys compiled into the CLI, for both tiers, after the tarball checksum (E124) and before anything is extracted. Split out of installer_locked.go (299-line ratchet).
// Inputs: the resolved registry manifest and whether the entry is licensed (paid).
// Outputs: nil (verified, or a one-line warning printed), or a coded error: E121 bad signature, E122 unknown key, E123 revoked key, E107 unsigned where signatures are required.
// Constraints: the release_signature covers the SOURCE tarball's sha256 (manifest.Checksum). A binary (platform) package on a pinned tier installs only when platform_signatures[<the platform the CLI selected>] verifies over the sha256 computed here from the downloaded archive (contract v1.1, P7-PLUG-84; never the registry's checksum field); absent or invalid is E121, with no source-signature fallback. The dev bypass never skips a present signature on a pinned tier. The manifest's author_public_key is never read for trust. --allow-unsigned is honoured for local path or link installs only; registry installs pass local=false and cannot opt out. A CLI plugin whose entry advertises a platform signature for this platform refuses the source-package fallback (E121, D-1415). Pure policy lives in verify/signature.Decide.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
)

// allowUnsignedEnv is how the command layer threads --allow-unsigned down, like
// NSELF_PLUGIN_INSTALL_TIER. It has no effect on a registry install.
const allowUnsignedEnv = "NSELF_PLUGIN_ALLOW_UNSIGNED"

// releaseTier maps the manifest's paid flag to the signing tier.
func releaseTier(paid bool) string {
	if paid {
		return signature.TierLicensed
	}
	return signature.TierFree
}

// unsignedAllowed reports whether an unsigned install may proceed because the
// caller asked for it: only a local path or link install honours the flag.
func unsignedAllowed(local bool) bool {
	return local && os.Getenv(allowUnsignedEnv) == "1"
}

// verifyReleaseSignature runs the signature decision for one registry
// manifest and prints its warning, if any.
//
// bypass is the audited dev bypass (NSELF_LICENSE_SKIP_VERIFY + FORCE, refused
// in prod/staging): it turns a MISSING signature, or any signature on an
// unpinned tier, into a skip. It never skips a present signature on a pinned
// tier: that is the trust root, and a flag named for licences must not open it.
//
// A binary package on a pinned tier is verified by verifyPlatformSignature
// (contract v1.1) and never by the source signature: the registry fields that
// choose a binary download (pluginType, binaryName, repository, platform
// checksums) are unsigned, so only a signature over the downloaded bytes and
// the CLI's own platform vouches for them. archivePath is the downloaded file.
func verifyReleaseSignature(manifest *PluginManifest, paid, local bool, artifactKind, archivePath string, bypass bool) error {
	tier := releaseTier(paid)
	pinned := signature.Pinned(tier)
	if pinned && artifactKind != "" && artifactKind != ArtifactKindSource {
		return verifyPlatformSignature(manifest, tier, artifactKind, archivePath, bypass)
	}
	if pinned && artifactKind == ArtifactKindSource {
		if err := refuseSourceFallback(manifest); err != nil {
			return err
		}
	}
	if bypass && (!pinned || manifest.ReleaseSignature == nil) {
		return nil
	}
	// compat.V15(P7-PLUG-23): a plugin with no verifiable release signature installs with a warning -> refused with E107
	v15 := compat.V15()
	out, err := signature.Decide(signature.Input{
		Tier:          tier,
		Slug:          manifest.Name,
		Version:       manifest.Version,
		SHA256:        strings.ToLower(manifest.Checksum),
		Release:       manifest.ReleaseSignature,
		V15:           v15,
		Strict:        pluginChecksumRequired(),
		AllowUnsigned: unsignedAllowed(local),
	})
	if err != nil {
		return err
	}
	if out.Warning != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", out.Warning)
	}
	return nil
}

// refuseSourceFallback refuses (E121) a source package for a CLI plugin whose
// entry advertises a platform signature for the platform the CLI selected.
// The entry's repository field is unsigned, so a 404 on the platform asset can
// be forced; without this check the install would drop to the source tarball,
// which only release_signature covers, and publish any binary it holds (D-1415).
// Not even the dev bypass skips it: the entry says a signed binary exists.
func refuseSourceFallback(manifest *PluginManifest) error {
	if len(cliBinaryNames(manifest.Name, manifest)) == 0 {
		return nil
	}
	platform, err := PlatformArch()
	if err != nil {
		return nil
	}
	if _, advertised := manifest.PlatformSignatures[platform]; !advertised {
		return nil
	}
	return errs.Newf("E121", "plugin %q: its entry advertises a signed %s binary package, but only the source package could be fetched; refusing the source fallback", manifest.Name, platform)
}

// verifyPlatformSignature checks the platform signature of a binary package
// on a pinned tier. Only the platform the CLI selected (artifactKind, from
// PlatformArch()) is ever looked up. The bypass skips an ABSENT entry, never a
// present one, whatever release_signature says. Everything else fails closed:
// no entry, no readable archive and a bad signature are all errors, and the
// install never falls back to the source package.
func verifyPlatformSignature(manifest *PluginManifest, tier, platform, archivePath string, bypass bool) error {
	rel, present := manifest.PlatformSignatures[platform]
	if !present {
		if bypass {
			return nil
		}
		return errs.Newf("E121", "plugin %q: the %s binary package has no platform signature", manifest.Name, platform)
	}
	sum, err := archiveSHA256(archivePath)
	if err != nil {
		return errs.Wrap("E121", fmt.Sprintf("plugin %q: cannot hash the %s binary package for its signature", manifest.Name, platform), err)
	}
	return signature.VerifyPlatform(tier, manifest.Name, manifest.Version, platform, sum, rel)
}

// archiveSHA256 returns the lowercase hex sha256 of the file at path.
func archiveSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
