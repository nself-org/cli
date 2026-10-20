package signature

// platform.go — verifies the release signature of a platform (binary) package.
//
// Purpose: contract plugins.release-signature v1.1 (hq PLUG D7, closes D-0840): each platform tarball is signed under its own domain, so a command-providing plugin can install on a pinned tier. The producer is plugins scripts/release/sign.sh (P7-PLUG-83).
// Inputs: tier, slug, version, the platform the CLI itself selected, the lowercase hex sha256 the CLI computed from the downloaded bytes, and the entry's platform_signatures[platform].
// Outputs: nil, or the same *errs.CLIError codes as Verify: E121 bad, malformed or missing signature, E122 unknown key (also a key of the other tier), E123 revoked key.
// Constraints: pure. The message is "nself-plugin-bin-v1\n<slug>\n<version>\n<platform>\n<sha256 hex>\n": its first line differs from the source domain, so a source signature never verifies as a platform signature and the reverse. The platform must be one of the five names. The check itself is verifyMessage, shared with Verify.

import "slices"

// Platforms are the platform names the release pipeline signs, as PlatformArch() returns them.
var Platforms = []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64"}

// ValidPlatform reports whether p is one of the five platform names.
func ValidPlatform(p string) bool { return slices.Contains(Platforms, p) }

// SigningInputPlatform returns the bytes the release pipeline signs for one
// platform tarball:
// "nself-plugin-bin-v1\n<slug>\n<version>\n<platform>\n<sha256 hex>\n".
func SigningInputPlatform(slug, version, platform, sha256hex string) []byte {
	return []byte("nself-plugin-bin-v1\n" + slug + "\n" + version + "\n" + platform + "\n" + sha256hex + "\n")
}

// VerifyPlatform checks rel for the given tier, slug, version, platform and
// platform-tarball sha256.
func VerifyPlatform(tier, slug, version, platform, sha256hex string, rel *Release) error {
	if !cleanField(slug) || !cleanField(version) || !ValidPlatform(platform) || !isLowerHex64(sha256hex) {
		return verifyMessage(tier, nil, rel, true)
	}
	return verifyMessage(tier, SigningInputPlatform(slug, version, platform, sha256hex), rel, false)
}
