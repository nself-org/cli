package compose

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// pgvectorMajorVersionRE extracts the leading numeric major version from a
// POSTGRES_VERSION string such as "16-alpine" or "16.4" so the pgvector image
// tag tracks the configured Postgres major version rather than being
// hardcoded to whatever the lock's pgvector entry currently pins.
var pgvectorMajorVersionRE = regexp.MustCompile(`^(\d+)`)

// ResolvePostgresImage decides which Postgres image `nself build` emits.
//
// Purpose: fix cli#384 — nself build previously ignored the pgvector pin
// entirely and always emitted postgres:<POSTGRES_VERSION>, silently swapping
// a running pgvector/pgvector container (uid 999, PGDATA subdir) for a plain
// alpine postgres image (uid 70, no PGDATA) on every regen. Restarting
// postgres under the swapped image on a populated database is a data-loss
// event (P1 EOP staging incident 2026-06-10).
// Inputs: pg — the resolved PostgresConfig (Image, Version, Extensions).
// Outputs: the image reference the postgres service should run.
// Constraints: precedence is fixed and MUST NOT be reordered:
//  1. pg.Image (POSTGRES_IMAGE), when set, always wins — an explicit
//     operator pin overrides every inference below it.
//  2. pg.Extensions containing "pgvector" (POSTGRES_EXTENSIONS) selects the
//     pgvector image matching the configured Postgres major version.
//  3. Otherwise, postgres:<POSTGRES_VERSION> (unchanged default behavior).
//
// buildPostgresService derives PGDATA and the container uid from the
// resulting image name (alpine vs. debian-family), so getting this
// precedence right also fixes PGDATA/uid preservation: a pgvector or other
// debian-family image always resolves to uid 999 + PGDATA, matching what the
// upstream image actually requires.
func ResolvePostgresImage(pg config.PostgresConfig) string {
	if img := strings.TrimSpace(pg.Image); img != "" {
		return img
	}
	if hasExtension(pg.Extensions, "pgvector") {
		return pgvectorImageForVersion(pg.Version)
	}
	return ImageRef("postgres", pg.Version)
}

// hasExtension reports whether extensions contains name, case-insensitively.
func hasExtension(extensions []string, name string) bool {
	for _, e := range extensions {
		if strings.EqualFold(strings.TrimSpace(e), name) {
			return true
		}
	}
	return false
}

// pgvectorImageForVersion maps a POSTGRES_VERSION like "16-alpine" or "16.4"
// to the matching pgvector/pgvector image tag. Falls back to the
// default pgvector entry (DefaultImage) when no leading major-version digit is found.
func pgvectorImageForVersion(version string) string {
	if major := pgvectorMajorVersionRE.FindString(version); major != "" {
		return ImageRef("pgvector", "pg"+major)
	}
	return DefaultImage("postgres")
}

// ImageDigests maps service name to sha256 digest for image pinning.
// When a digest is available, ResolveImage appends @sha256:... to the tag.
// Populated by LoadImageDigests from the project config directory.
var ImageDigests = map[string]string{}

// DigestConfigFile is the filename where image digests are stored.
const DigestConfigFile = ".nself-image-digests.json"

// DefaultImage returns the default reference for a service when the caller
// supplies none: the lock entry rendered in the active pinning mode. The
// postgres service falls back to the pgvector entry (the pre-lock pin), which is
// only reachable for an unparseable POSTGRES_VERSION.
func DefaultImage(service string) string {
	if service == "postgres" {
		service = "pgvector"
	}
	return ImageRef(service, "")
}

// ResolveImage returns the image reference for a service, optionally with a
// sha256 digest suffix when a project digest is recorded.
//
// Precedence: a non-empty caller-supplied image (built from env/config such as
// POSTGRES_VERSION / HASURA_VERSION via ImageRef, or ResolvePostgresImage for
// postgres) ALWAYS wins. DefaultImage applies only when the caller supplies no
// image. A reference that already carries a digest (lock mode) is left alone.
func ResolveImage(service, image string) string {
	resolved := image
	if resolved == "" {
		resolved = DefaultImage(service)
	}
	if strings.Contains(resolved, "@sha256:") {
		return resolved
	}
	// Append digest if available for this service.
	if digest, ok := ImageDigests[service]; ok && digest != "" {
		resolved = resolved + "@sha256:" + digest
	}
	return resolved
}

// LoadImageDigests reads digest pins from the project config directory.
// Missing file is not an error (digests are opt-in via `nself update images`).
func LoadImageDigests(projectDir string) error {
	path := filepath.Join(projectDir, DigestConfigFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading image digests: %w", err)
	}
	var digests map[string]string
	if err := json.Unmarshal(data, &digests); err != nil {
		return fmt.Errorf("parsing image digests: %w", err)
	}
	ImageDigests = digests
	return nil
}

// SaveImageDigests writes digest pins to the project config directory.
func SaveImageDigests(projectDir string, digests map[string]string) error {
	path := filepath.Join(projectDir, DigestConfigFile)
	data, err := json.MarshalIndent(digests, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling image digests: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing image digests: %w", err)
	}
	return nil
}
