package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var updateImagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Refresh Docker image digests from registries",
	Long: `Fetch the latest sha256 digests for all pinned Docker images and
store them in the project directory. On the next 'nself build', images
will be pinned to exact digests for reproducible deployments.

Use --dry-run to preview changes without writing.`,
	RunE: runUpdateImages,
}

func init() {
	updateImagesCmd.Flags().Bool("dry-run", false, "Show digest changes without writing")
	updateCmd.AddCommand(updateImagesCmd)
}

func runUpdateImages(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	ui.CommandHeader("nself update images", "Refresh Docker image digests")

	workdir, err := resolveProjectDir()
	if err != nil {
		return fmt.Errorf("no nself project found: %w", err)
	}

	// Load existing digests for diff display.
	if loadErr := compose.LoadImageDigests(workdir); loadErr != nil {
		ui.Warn(fmt.Sprintf("Could not load existing digests: %v", loadErr))
	}
	oldDigests := make(map[string]string)
	for k, v := range compose.ImageDigests {
		oldDigests[k] = v
	}

	// Fetch digests for all default images.
	newDigests := make(map[string]string)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Every core and optional lock entry is a service `nself build` can emit.
	// The digest is looked up for the reference the project runs by default
	// (legacy_ref), keyed by the service name ResolveImage uses.
	for _, locked := range compose.LockedImages() {
		service, image := locked.Name, locked.LegacyRef
		// Skip our own image — always uses :latest.
		if (locked.Role != compose.RoleCore && locked.Role != compose.RoleOptional) || service == "admin" {
			continue
		}

		digest, fetchErr := fetchImageDigest(ctx, image)
		if fetchErr != nil {
			ui.Warn(fmt.Sprintf("  %s: failed to fetch digest (%v)", service, fetchErr))
			continue
		}
		newDigests[service] = digest

		changed := oldDigests[service] != digest
		status := "unchanged"
		if changed {
			status = "updated"
		}
		if _, hadOld := oldDigests[service]; !hadOld {
			status = "new"
		}

		fmt.Printf("  %-16s %s [%s]\n", service, digest[:16]+"...", status)
	}

	if dryRun {
		ui.Info("Dry run — no changes written.")
		return nil
	}

	if err := compose.SaveImageDigests(workdir, newDigests); err != nil {
		return fmt.Errorf("saving digests: %w", err)
	}

	ui.Success(fmt.Sprintf("Saved %d image digests to %s", len(newDigests), compose.DigestConfigFile))
	ui.Info("Run 'nself build' to regenerate docker-compose.yml with pinned digests.")
	return nil
}

// fetchImageDigest uses `docker manifest inspect` to get the sha256 digest
// for an image reference. Returns the hex digest without the "sha256:" prefix.
func fetchImageDigest(ctx context.Context, image string) (string, error) {
	out, err := docker.ManifestInspect(ctx, image, true)
	if err != nil {
		return "", fmt.Errorf("docker manifest inspect %s: %w", image, err)
	}

	// Parse the manifest JSON to extract the digest.
	// docker manifest inspect --verbose returns either a single object or an array.
	type manifestEntry struct {
		Descriptor struct {
			Digest string `json:"digest"`
		} `json:"Descriptor"`
	}

	raw := strings.TrimSpace(string(out))

	// Try array first (multi-platform).
	if strings.HasPrefix(raw, "[") {
		var entries []manifestEntry
		if err := json.Unmarshal([]byte(raw), &entries); err == nil && len(entries) > 0 {
			digest := entries[0].Descriptor.Digest
			return strings.TrimPrefix(digest, "sha256:"), nil
		}
	}

	// Try single object.
	var entry manifestEntry
	if err := json.Unmarshal([]byte(raw), &entry); err == nil && entry.Descriptor.Digest != "" {
		return strings.TrimPrefix(entry.Descriptor.Digest, "sha256:"), nil
	}

	// Fallback: look for digest in the raw output.
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "sha256:") {
			parts := strings.Split(line, "sha256:")
			if len(parts) >= 2 {
				digest := strings.Trim(parts[1], `",`)
				if len(digest) == 64 {
					return digest, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no digest found in manifest for %s", image)
}
