// Package autoinstall holds the verified-install receipt logic that was
// previously in cmd/commands. Moving it here keeps cmd/commands under its
// package-size ceiling while keeping the trust checks next to the plugin
// verification code they depend on.
package autoinstall

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
)

// Receipt binds a verified install to the complete catalogue entry (including
// signing key and signatures) and to the published bytes. Old/manual installs
// have no receipt and refresh.
type Receipt struct {
	Manifest json.RawMessage `json:"manifest"`
	Digest   [32]byte        `json:"digest"`
}

// Cached returns the published binary path when the on-disk receipt still
// matches the current catalogue entry and the binary bytes. It re-runs trust
// policy with this CLI's pinned/revoked keys on every use.
func Cached(dir, slug string, m *plugin.PluginManifest) (string, bool) {
	if _, err := signature.Decide(signature.Input{Tier: signature.TierFree, Slug: m.Name, Version: m.Version, SHA256: strings.ToLower(m.Checksum), Release: m.ReleaseSignature, Strict: true}); err != nil {
		return "", false
	}
	platform, err := plugin.PlatformArch()
	if err != nil {
		return "", false
	}
	rel, ok := m.PlatformSignatures[platform]
	if !ok || signature.VerifyPlatform(signature.TierFree, m.Name, m.Version, platform, m.PlatformChecksums[platform], rel) != nil {
		return "", false
	}
	bin := plugin.PublishedBinaryPath(BinaryName(slug, m))
	if !isRegularFile(bin) || ReadInstalledVersion(dir, slug) != m.Version {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(dir, slug, ".autoinstall-receipt.json"))
	if err != nil {
		return "", false
	}
	var r Receipt
	if json.Unmarshal(data, &r) != nil {
		return "", false
	}
	manifest, err := json.Marshal(m)
	if err != nil || !bytes.Equal(manifest, r.Manifest) {
		return "", false
	}
	binary, err := os.ReadFile(bin)
	return bin, err == nil && sha256.Sum256(binary) == r.Digest
}

// Record writes a receipt for the just-installed binary so subsequent runs
// can skip the download when the catalogue entry and bytes are unchanged.
func Record(dir, slug string, m *plugin.PluginManifest, bin string) error {
	binary, err := os.ReadFile(bin)
	if err != nil {
		return err
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		return err
	}
	data, err := json.Marshal(Receipt{Manifest: manifest, Digest: sha256.Sum256(binary)})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, slug, ".autoinstall-receipt.json"), data, 0600)
}

// BinaryName is the name the installer publishes a single-command plugin
// under: the manifest's binaryName, else nself-<slug>.
func BinaryName(slug string, m *plugin.PluginManifest) string {
	if m.BinaryName != "" {
		return m.BinaryName
	}
	return "nself-" + slug
}

func isRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// ReadInstalledVersion returns the version in <pluginDir>/<slug>/plugin.json, or "".
func ReadInstalledVersion(pluginDir, slug string) string {
	b, err := os.ReadFile(filepath.Join(pluginDir, slug, "plugin.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return m.Version
}
