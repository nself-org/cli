package commands

// verify_sbom.go — `nself verify-sbom`
//
// Downloads the CycloneDX SBOM for a given CLI release from GitHub Releases
// and verifies the cosign bundle signature. Prints VERIFIED on success.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/spf13/cobra"
)

const sbomGitHubRepo = "nself-org/cli"

var verifySBOMCmd = &cobra.Command{
	Use:   "verify-sbom",
	Short: "Verify the SBOM signature for a CLI release",
	Long: `Download and verify the CycloneDX SBOM attached to a GitHub Release.

The command downloads sbom-cli-{version}.cdx.json and its cosign bundle from
GitHub Releases, then runs cosign verify-blob to confirm the signature.

Requirements:
  - cosign must be installed and in PATH (brew install cosign)
  - version must correspond to a published GitHub Release

Examples:
  nself verify-sbom --version v1.0.9
  nself verify-sbom --version v1.0.10 --repo nself-org/cli`,
	RunE: runVerifySBOM,
}

func runVerifySBOM(cmd *cobra.Command, args []string) error {
	version, _ := cmd.Flags().GetString("version")
	if version == "" {
		return fmt.Errorf("--version is required (e.g. --version v1.0.9)")
	}
	repo, _ := cmd.Flags().GetString("repo")
	if repo == "" {
		repo = sbomGitHubRepo
	}

	// Check cosign is available.
	if _, err := exec.LookPath("cosign"); err != nil {
		return fmt.Errorf("cosign not found in PATH — install with: brew install cosign")
	}

	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, version)
	sbomFile := fmt.Sprintf("sbom-cli-%s.cdx.json", version)
	bundleFile := fmt.Sprintf("sbom-cli-%s.cdx.json.bundle", version)

	tmpDir, err := os.MkdirTemp("", "nself-sbom-verify-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	sbomPath := filepath.Join(tmpDir, sbomFile)
	bundlePath := filepath.Join(tmpDir, bundleFile)

	fmt.Printf("Downloading SBOM for %s...\n", version)
	if err := sbomDownloadFile(cmd.Context(), baseURL+"/"+sbomFile, sbomPath); err != nil {
		return fmt.Errorf("downloading SBOM: %w", err)
	}
	if err := sbomDownloadFile(cmd.Context(), baseURL+"/"+bundleFile, bundlePath); err != nil {
		return fmt.Errorf("downloading cosign bundle: %w", err)
	}

	fmt.Println("Verifying cosign bundle signature...")
	c := exec.Command("cosign", "verify-blob",
		sbomPath,
		"--bundle", bundlePath,
		"--certificate-identity-regexp", fmt.Sprintf("^https://github.com/%s/", repo),
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com",
	)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("cosign verification FAILED: %w", err)
	}

	setData(cmd, verifySBOMData{Version: version, Repo: repo, Verified: true})
	fmt.Printf("\nSBOM for %s: VERIFIED\n", version)
	return nil
}

// sbomDownloadFile fetches url and writes to dest.
func sbomDownloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	resp, err := httptimeout.Installer.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, resp.Body)
	return err
}

func init() {
	verifySBOMCmd.Flags().String("version", "", "Release version to verify (e.g. v1.0.9)")
	verifySBOMCmd.Flags().String("repo", sbomGitHubRepo, "GitHub repo (owner/name)")

	RootCmd.AddCommand(verifySBOMCmd)
}
