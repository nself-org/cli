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

// AdminImagePath is the Docker Hub image name for the nSelf Admin GUI service.
// It uses the Docker Hub nself/ namespace (NOT GitHub Container Registry).
// Intentionally referencing nself/nself-admin (Docker Hub) — never github.com/nself-org/ paths.
const AdminImagePath = "nself/nself-admin"

// MinioImagePath is the registry path for the MinIO object-storage image.
//
// It is REGISTRY-QUALIFIED and must stay that way. MinIO removed the
// `minio/minio` repository from Docker Hub: as of 2026-09-14 the Hub API
// returns `{"message":"object not found"}` for it and every tag — including
// long-published pins such as RELEASE.2024-01-16T16-07-38Z — answers 401 to
// both anonymous and authenticated manifest requests. A `docker pull` of it
// fails with "pull access denied for minio/minio, repository does not exist
// or may require 'docker login'", so every generated stack with
// MINIO_ENABLED=true could no longer start. Authenticating does NOT help;
// the repository is gone, not gated.
//
// quay.io/minio/minio is MinIO's own registry and serves the same tags
// anonymously (verified 2026-09-14: :latest, RELEASE.2024-01-16T16-07-38Z
// and RELEASE.2024-10-02T17-50-41Z all return 200).
//
// Both the DefaultImageVersions pin below and buildMinioService's
// MINIO_VERSION path must build from this constant so the two cannot drift
// back to an unqualified Docker Hub name.
const MinioImagePath = "quay.io/minio/minio"

// DefaultImageVersions maps service name to pinned image:tag.
// Update with each nSelf release.
var DefaultImageVersions = map[string]string{
	"postgres": "pgvector/pgvector:pg16",
	"hasura":   "hasura/graphql-engine:v2.44.0",
	"auth":     "nhost/hasura-auth:0.36.0",
	"nginx":    "nginx:1.25-alpine",
	"redis":    "redis:7.2-alpine",
	"minio":    MinioImagePath + ":RELEASE.2024-01-16T16-07-38Z",
	// nhost/functions:0.3.7 never existed. nhost's 0.x line stops at 0.1.9 and
	// the repository now tags as <node-major>-<version> (22-2.2.0, 26-2.2.0);
	// `docker manifest inspect nhost/functions:0.3.7` answers "no such
	// manifest". Nothing broke because this entry is unreachable in practice:
	// applyDefaultsFunctions sets FUNCTIONS_VERSION to "latest" when unset, so
	// buildFunctionsService always passes a non-empty image and ResolveImage
	// never falls back to this pin. "latest" is therefore what users actually
	// run, and naming it here makes the pin honest without changing any
	// emitted compose file. Choosing a real pinned tag is an upgrade decision
	// (latest is not any of the current 2.2.0 tags), not a drive-by edit.
	"functions":   "nhost/functions:latest",
	"mailpit":     "axllent/mailpit:v1.15",
	"meilisearch": "getmeili/meilisearch:v1.6",
	"typesense":   "typesense/typesense:0.25.2",
	"admin":       AdminImagePath + ":latest", // intentionally latest — our own image
	"mlflow":      "ghcr.io/mlflow/mlflow:v2.10.0",
}

// pgvectorMajorVersionRE extracts the leading numeric major version from a
// POSTGRES_VERSION string such as "16-alpine" or "16.4" so the pgvector image
// tag tracks the configured Postgres major version rather than being
// hardcoded to whatever DefaultImageVersions currently pins.
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
	return fmt.Sprintf("postgres:%s", pg.Version)
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
// DefaultImageVersions pin when no leading major-version digit is found.
func pgvectorImageForVersion(version string) string {
	if major := pgvectorMajorVersionRE.FindString(version); major != "" {
		return fmt.Sprintf("pgvector/pgvector:pg%s", major)
	}
	return DefaultImageVersions["postgres"]
}

// ImageDigests maps service name to sha256 digest for image pinning.
// When a digest is available, ResolveImage appends @sha256:... to the tag.
// Populated by LoadImageDigests from the project config directory.
var ImageDigests = map[string]string{}

// DigestConfigFile is the filename where image digests are stored.
const DigestConfigFile = ".nself-image-digests.json"

// ResolveImage returns the image tag for a service, optionally with a sha256
// digest suffix when available.
//
// Precedence: a non-empty caller-supplied image (built from env/config such as
// POSTGRES_VERSION / HASURA_VERSION, or ResolvePostgresImage for postgres)
// ALWAYS wins. The DefaultImageVersions pin applies only when the caller
// supplies no image — for postgres specifically that image is always
// resolved via ResolvePostgresImage before reaching here, so this pin is a
// fallback for other services (or an unparseable POSTGRES_VERSION), not the
// live postgres selection path.
func ResolveImage(service, image string) string {
	resolved := image
	if resolved == "" {
		if pinned, ok := DefaultImageVersions[service]; ok {
			resolved = pinned
		}
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
