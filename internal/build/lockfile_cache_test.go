package build

// lockfile_cache_test.go — an existing project never moves to Valkey
// implicitly (P7-LIVE-19, EPIC D16, review F14).
//
// Purpose: a v1.5 build with no lock and no REDIS_ENGINE must keep the cache
// engine the project's existing docker-compose.yml shows, in whatever form the
// image is written (a literal tag, or a ${VAR:-default} reference that the lock
// bootstrap cannot pin), and move only a project with no prior cache evidence.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/lock"
)

// cacheOf builds a v1.5 project whose env is extra and whose existing compose
// is existing ("" = none), and returns the generated redis service's image and
// first volume.
func cacheOf(t *testing.T, existing, extraEnv string) (image, volume string) {
	t.Helper()
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	envPath := filepath.Join(f.workdir, ".env")
	raw, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, append(raw, []byte("\nREDIS_ENABLED=true\n"+extraEnv)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if existing != "" {
		withExistingCompose(t, f, existing)
	}
	lockBuild(t, f)
	out, err := os.ReadFile(filepath.Join(f.workdir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	imgs, err := lock.ComposeImages(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "redis_data:/data") {
			volume = "redis_data"
		}
		if strings.Contains(line, "valkey_data:/data") {
			volume = "valkey_data"
		}
	}
	return imgs["redis"], volume
}

func TestBuildExistingRedisProjectKeepsRedis(t *testing.T) {
	for _, tc := range []struct{ name, compose string }{
		{"literal tag", "services:\n  redis:\n    image: redis:7-alpine\n"},
		{"variable image", "services:\n  redis:\n    image: redis:${REDIS_VERSION:-7-alpine}\n"},
		{"variable whole ref", "services:\n  redis:\n    image: ${REDIS_IMAGE:-redis:7-alpine}\n"},
		{"redis_data volume only", "services:\n  postgres:\n    image: postgres:16-alpine\nvolumes:\n  redis_data: {}\n"},
		{"redis service built locally", "services:\n  redis:\n    build: ./redis\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img, vol := cacheOf(t, tc.compose, "")
			if strings.Contains(img, "valkey") || vol != "redis_data" {
				t.Fatalf("an existing redis project moved: image %q volume %q", img, vol)
			}
		})
	}
}

// Controls: no cache evidence defaults to valkey; an existing valkey project
// stays valkey; the explicit key still wins over what the compose shows.
func TestBuildCacheEngineControls(t *testing.T) {
	if img, vol := cacheOf(t, "", ""); !strings.Contains(img, "valkey") || vol != "valkey_data" {
		t.Errorf("new project: image %q volume %q, want valkey", img, vol)
	}
	if img, vol := cacheOf(t, "services:\n  postgres:\n    image: postgres:16-alpine\n", ""); !strings.Contains(img, "valkey") || vol != "valkey_data" {
		t.Errorf("compose without a cache: image %q volume %q, want valkey", img, vol)
	}
	if img, vol := cacheOf(t, "services:\n  redis:\n    image: valkey/valkey:8.1-alpine\n", ""); !strings.Contains(img, "valkey") || vol != "valkey_data" {
		t.Errorf("existing valkey: image %q volume %q", img, vol)
	}
	if img, vol := cacheOf(t, "services:\n  redis:\n    image: redis:${REDIS_VERSION:-7-alpine}\n", "REDIS_ENGINE=valkey\n"); !strings.Contains(img, "valkey") || vol != "valkey_data" {
		t.Errorf("explicit valkey over an existing redis: image %q volume %q", img, vol)
	}
}
