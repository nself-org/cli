package commands

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

// A receipt is written only after the verified seam publishes a binary. Bind
// it to the complete catalogue entry (including signing key and signatures)
// and to the published bytes. Old/manual installs have no receipt and refresh.
type autoInstallReceipt struct {
	Manifest json.RawMessage `json:"manifest"`
	Digest   [32]byte        `json:"digest"`
}

func autoInstallCached(dir, slug string, m *plugin.PluginManifest) (string, bool) {
	// Re-run trust policy with this CLI's pinned/revoked keys on every use.
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
	bin := plugin.PublishedBinaryPath(autoInstallBinaryName(slug, m))
	if !isRegularFile(bin) || readInstalledVersion(dir, slug) != m.Version {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(dir, slug, ".autoinstall-receipt.json"))
	if err != nil {
		return "", false
	}
	var receipt autoInstallReceipt
	if json.Unmarshal(data, &receipt) != nil {
		return "", false
	}
	manifest, err := json.Marshal(m)
	if err != nil || !bytes.Equal(manifest, receipt.Manifest) {
		return "", false
	}
	binary, err := os.ReadFile(bin)
	return bin, err == nil && sha256.Sum256(binary) == receipt.Digest
}

func autoInstallRecord(dir, slug string, m *plugin.PluginManifest, bin string) error {
	binary, err := os.ReadFile(bin)
	if err != nil {
		return err
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		return err
	}
	data, err := json.Marshal(autoInstallReceipt{Manifest: manifest, Digest: sha256.Sum256(binary)})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, slug, ".autoinstall-receipt.json"), data, 0600)
}
