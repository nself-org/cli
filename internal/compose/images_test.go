package compose

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestResolveImage_KnownService(t *testing.T) {
	// Explicit caller-supplied image (env/config driven) wins over the pin.
	// Pins overriding configured versions force-migrated existing deployments
	// (P1 EOP staging incident 2026-06-10).
	got := ResolveImage("postgres", "postgres:15")
	want := "postgres:15"
	if got != want {
		t.Errorf("ResolveImage(postgres, explicit) = %q, want %q", got, want)
	}
	// Pin applies only when no image is supplied.
	got = ResolveImage("postgres", "")
	want = "pgvector/pgvector:pg16"
	if got != want {
		t.Errorf("ResolveImage(postgres, empty) = %q, want %q", got, want)
	}
}

func TestResolveImage_UnknownService(t *testing.T) {
	got := ResolveImage("unknown-svc", "my-image:v1")
	want := "my-image:v1"
	if got != want {
		t.Errorf("ResolveImage(unknown) = %q, want %q", got, want)
	}
}

func TestResolveImage_AdminIsLatest(t *testing.T) {
	// Explicit image wins; the :latest pin applies only for empty input.
	got := ResolveImage("admin", "nself/nself-admin:v1.0")
	want := "nself/nself-admin:v1.0"
	if got != want {
		t.Errorf("ResolveImage(admin, explicit) = %q, want %q", got, want)
	}
	got = ResolveImage("admin", "")
	want = "nself/nself-admin:latest"
	if got != want {
		t.Errorf("ResolveImage(admin, empty) = %q, want %q", got, want)
	}
}

// TestAdminImage_DockerHubNotGHCR verifies the admin image entry uses
// the Docker Hub nself/ namespace and never github.com/nself-org/ paths.
// This closes C1-01 from the Dim 2 undocumented dependency audit (S02.T-UNDEP-01).
func TestAdminImage_DockerHubNotGHCR(t *testing.T) {
	r, ok := LockedRef("admin")
	if !ok {
		t.Fatal("lock has no admin entry")
	}
	if strings.Contains(r.Repository, "github.com") || strings.Contains(r.Repository, "ghcr.io") {
		t.Errorf("admin repository %q must be Docker Hub, never GitHub paths", r.Repository)
	}
	if !strings.HasPrefix(r.Repository, "docker.io/nself/") || !strings.HasPrefix(r.LegacyRef, "nself/") {
		t.Errorf("admin = %q / legacy %q, want the Docker Hub nself/ namespace", r.Repository, r.LegacyRef)
	}
}

// TestResolvePostgresImage_ExplicitImageWins verifies precedence branch 1:
// POSTGRES_IMAGE always wins, even when POSTGRES_EXTENSIONS also lists
// pgvector or POSTGRES_VERSION is set (cli#384).
func TestResolvePostgresImage_ExplicitImageWins(t *testing.T) {
	pg := config.PostgresConfig{
		Image:      "pgvector/pgvector:pg16",
		Version:    "16-alpine",
		Extensions: []string{"pgvector"},
	}
	got := ResolvePostgresImage(pg)
	want := "pgvector/pgvector:pg16"
	if got != want {
		t.Errorf("ResolvePostgresImage(explicit image) = %q, want %q", got, want)
	}
}

// TestResolvePostgresImage_PgvectorExtensionImpliesImage verifies precedence
// branch 2: POSTGRES_EXTENSIONS containing pgvector selects the pgvector
// image matching the configured major version when POSTGRES_IMAGE is unset.
// This is the defect at the heart of cli#384 — this pin was previously dead
// code under buildPostgresService's old precedence.
func TestResolvePostgresImage_PgvectorExtensionImpliesImage(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"alpine suffix", "16-alpine", "pgvector/pgvector:pg16"},
		{"dotted version", "15.4", "pgvector/pgvector:pg15"},
		{"unparseable falls back to pin", "latest", "pgvector/pgvector:pg16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg := config.PostgresConfig{Version: tt.version, Extensions: []string{"pgvector"}}
			got := ResolvePostgresImage(pg)
			if got != tt.want {
				t.Errorf("ResolvePostgresImage(version=%q, pgvector) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

// TestResolvePostgresImage_ExtensionMatchCaseInsensitiveAndTrimmed verifies
// POSTGRES_EXTENSIONS entries are matched case-insensitively and tolerate
// surrounding whitespace from a comma-separated env value.
func TestResolvePostgresImage_ExtensionMatchCaseInsensitiveAndTrimmed(t *testing.T) {
	pg := config.PostgresConfig{
		Version:    "16-alpine",
		Extensions: []string{"uuid-ossp", " PgVector ", "pgcrypto"},
	}
	got := ResolvePostgresImage(pg)
	want := "pgvector/pgvector:pg16"
	if got != want {
		t.Errorf("ResolvePostgresImage(mixed-case extension) = %q, want %q", got, want)
	}
}

// TestResolvePostgresImage_DefaultVersionOnly verifies precedence branch 3:
// with no POSTGRES_IMAGE and no pgvector extension, the plain
// postgres:<POSTGRES_VERSION> default is unchanged from before cli#384.
func TestResolvePostgresImage_DefaultVersionOnly(t *testing.T) {
	pg := config.PostgresConfig{Version: "16-alpine", Extensions: []string{"uuid-ossp", "pgcrypto"}}
	got := ResolvePostgresImage(pg)
	want := "postgres:16-alpine"
	if got != want {
		t.Errorf("ResolvePostgresImage(no image, no pgvector) = %q, want %q", got, want)
	}
}

// TestLockedImages_NoGoModulePaths asserts that no locked image contains a Go
// module-style path (github.com/nself-org/). Docker references use
// registry/org/image format, never Go module paths.
func TestLockedImages_NoGoModulePaths(t *testing.T) {
	for _, r := range LockedImages() {
		if strings.Contains(r.Repository, "github.com/nself-org/") || strings.Contains(r.LegacyRef, "github.com/nself-org/") {
			t.Errorf("image %q: %q contains Go module path github.com/nself-org/; use Docker registry format", r.Name, r.Repository)
		}
	}
}

// TestMinioImageIsRegistryQualified guards the fix for the 2026-09-14 storage
// outage: MinIO deleted the `minio/minio` repository from Docker Hub, so an
// unqualified reference resolves to a repository that no longer exists and
// every generated stack with MINIO_ENABLED=true failed to pull. The lock entry
// and buildMinioService's MINIO_VERSION path must stay on the maintained
// registry path; a bare "minio/minio:..." is the regression this catches.
func TestMinioImageIsRegistryQualified(t *testing.T) {
	r, ok := LockedRef("minio")
	if !ok {
		t.Fatal(`lock has no "minio" entry`)
	}
	if r.Repository != "docker.io/pgsty/minio" || !strings.HasPrefix(r.LegacyRef, "docker.io/pgsty/minio:") {
		t.Errorf("minio = %q / legacy %q, want docker.io/pgsty/minio", r.Repository, r.LegacyRef)
	}
}

// TestBuildMinioService_UsesPinnedRegistry covers the MINIO_VERSION path (legacy
// mode): an explicit version and the empty-version default are both checked.
func TestBuildMinioService_UsesPinnedRegistry(t *testing.T) {
	t.Setenv(EnvImagePinning, PinningLegacy)
	for _, tc := range []struct {
		name    string
		version string
		want    string
	}{
		{"explicit version", "RELEASE.2026-06-18T00-00-00Z", "docker.io/pgsty/minio:RELEASE.2026-06-18T00-00-00Z"},
		{"empty version defaults to latest", "", "docker.io/pgsty/minio:latest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &Generator{cfg: &config.Config{
				ProjectName:   "testproject",
				DockerNetwork: "testproject_network",
				Minio:         config.MinioConfig{Enabled: true, Version: tc.version},
			}}
			if got := g.buildMinioService().Image; got != tc.want {
				t.Errorf("buildMinioService().Image = %q, want %q", got, tc.want)
			}
		})
	}
}
