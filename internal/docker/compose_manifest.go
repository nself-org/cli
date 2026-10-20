package docker

// compose_manifest.go: the compose file list `nself build` records, readable by
// packages that may not import internal/build (service lifecycle, D-0703).
// Inputs: a project root. Outputs: the ordered -f list, the hand-written
// override and the image override last. internal/build wraps these functions.
// Constraints: one implementation; build's ReadComposeManifest delegates here.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ComposeManifestFile is the path relative to the project root of the ordered
// compose file list `nself build` writes.
const ComposeManifestFile = ".nself/compose-files.txt"

// ReadComposeManifest reads .nself/compose-files.txt and returns the ordered
// list of compose file paths. If the manifest does not exist, it returns a
// single-element slice with "docker-compose.yml" (relative, base only) so
// callers always get a usable default. Lines pointing to files that no longer
// exist on disk (e.g. a plugin was uninstalled) are silently skipped.
func ReadComposeManifest(workdir string) ([]string, error) {
	manifestPath := filepath.Join(workdir, ComposeManifestFile)

	f, err := os.Open(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return WithUserOverride(workdir, []string{"docker-compose.yml"}), nil
		}
		return nil, fmt.Errorf("opening compose manifest: %w", err)
	}
	defer func() { _ = f.Close() }()

	var paths []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Skip lines pointing to files that no longer exist.
		if _, err := os.Stat(line); err != nil {
			continue
		}
		paths = append(paths, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading compose manifest: %w", err)
	}

	// If the manifest was empty or all entries were stale, fall back to base.
	if len(paths) == 0 {
		return WithUserOverride(workdir, []string{"docker-compose.yml"}), nil
	}

	return WithUserOverride(workdir, paths), nil
}

// UserComposeOverrideFile is the hand-written file Docker Compose merges
// automatically when it is invoked with no -f at all.
const UserComposeOverrideFile = "docker-compose.override.yml"

// WithUserOverride appends the user's docker-compose.override.yml when it sits
// next to the generated compose file.
//
// `docker compose` only auto-merges the override when invoked with NO -f flag.
// The CLI always passes -f (it has to: plugin fragments are separate files), so
// the override was silently inert on every project — the container came up with
// none of it applied and nothing said so. Reported by the ntask clean-fork
// self-host drill, 2026-08-24, where the override was the only place the
// functions service got its entrypoint, a writable rootfs and its SMTP/MinIO
// environment.
//
// It is appended LAST on purpose: Compose lets later -f files win, and a
// hand-written override must beat both the generated base and any plugin
// fragment, which is the whole reason it exists.
func WithUserOverride(workdir string, paths []string) []string {
	userListed := false
	for _, p := range paths {
		if filepath.Base(p) == UserComposeOverrideFile {
			userListed = true
		}
	}
	if !userListed {
		if _, err := os.Stat(filepath.Join(workdir, UserComposeOverrideFile)); err == nil {
			paths = append(paths, UserComposeOverrideFile)
		}
	}
	for _, p := range paths {
		if filepath.Clean(p) == filepath.Join(workdir, ImageOverrideFile) || p == ImageOverrideFile {
			return paths
		}
	}
	if _, err := os.Stat(filepath.Join(workdir, ImageOverrideFile)); err == nil {
		paths = append(paths, filepath.Join(workdir, ImageOverrideFile))
	}
	return paths
}
