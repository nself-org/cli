package build

import (
	"strings"
	"testing"
)

// TestBuildCacheImageEvidence prevents an unpinned rebuild changing data volumes.
func TestBuildCacheImageEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, image, env, mount, engine string
	}{
		{"overridden fallback", "${CACHE_IMAGE:-valkey/valkey:8.1-alpine}", "redis:7-alpine", "valkey_data:/data", "redis"},
		{"variable only", "${CACHE_IMAGE}", "private/cache:stable", "valkey_data:/data", "valkey"},
		{"long mount", "${CACHE_IMAGE}", "private/cache:stable", "", "valkey"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CACHE_IMAGE", tc.env)
			mount := "      - " + tc.mount + "\n"
			if tc.mount == "" {
				mount = "      - type: volume\n        source: valkey_data\n        target: /data\n"
			}
			existing := "services:\n  redis:\n    image: " + tc.image + "\n    volumes:\n" + mount + "volumes:\n  redis_data: {}\n  valkey_data: {}\n"
			image, volume := cacheOf(t, existing, "CACHE_IMAGE="+tc.env+"\n")
			if !strings.Contains(image, tc.engine) || volume != tc.engine+"_data" {
				t.Fatalf("image %q, volume %q; want %s with its existing data volume", image, volume, tc.engine)
			}
		})
	}
}
