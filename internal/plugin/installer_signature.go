package plugin

// installer_signature.go — Step 5b of the install sequence: release-signature verification.
//
// Purpose: verify the registry's release_signature (hq PLUG D7, contract plugins.release-signature v1) against the keys compiled into the CLI, for both tiers, after the tarball checksum (E124) and before anything is extracted. Split out of installer_locked.go (299-line ratchet).
// Inputs: the resolved registry manifest and whether the entry is licensed (paid).
// Outputs: nil (verified, or a one-line warning printed), or a coded error: E121 bad signature, E122 unknown key, E123 revoked key, E107 unsigned where signatures are required.
// Constraints: the signature covers the SOURCE tarball's sha256 (manifest.Checksum); a binary tarball is held to platform_checksums and the same signed release row. The manifest's author_public_key is never read for trust. --allow-unsigned is honoured for local path or link installs only; registry installs pass local=false and cannot opt out. Pure policy lives in verify/signature.Decide.

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compat"
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
func verifyReleaseSignature(manifest *PluginManifest, paid, local bool) error {
	// compat.V15(P7-PLUG-23): a plugin with no verifiable release signature installs with a warning -> refused with E107
	v15 := compat.V15()
	out, err := signature.Decide(signature.Input{
		Tier:          releaseTier(paid),
		Slug:          manifest.Name,
		Version:       manifest.Version,
		SHA256:        strings.ToLower(manifest.Checksum),
		Release:       manifest.ReleaseSignature,
		V15:           v15,
		Strict:        pluginChecksumRequired() && EffectiveStatus(manifest.PublishStatus) == "stable",
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
