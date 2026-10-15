package compose

import "testing"

func TestCacheEvidenceInterpolation(t *testing.T) {
	t.Setenv("CACHE_IMAGE", "redis:7-alpine")
	t.Setenv("EMPTY_CACHE_IMAGE", "")
	for _, tc := range []struct{ image, want string }{
		{"${CACHE_IMAGE:-valkey/valkey:8.1-alpine}", EngineRedis},
		{"$CACHE_IMAGE", EngineRedis},
		{"${EMPTY_CACHE_IMAGE:-valkey/valkey:8.1-alpine}", EngineValkey},
		{"${EMPTY_CACHE_IMAGE-valkey/valkey:8.1-alpine}", EngineRedis},
		{"${CACHE_IMAGE:+valkey/valkey:8.1-alpine}", EngineValkey},
		{"${EMPTY_CACHE_IMAGE+valkey/valkey:8.1-alpine}", EngineValkey},
		{"${EMPTY_CACHE_IMAGE:-${CACHE_IMAGE}}", EngineRedis},
	} {
		t.Run(tc.image, func(t *testing.T) {
			if got := ExistingCacheEngine([]byte("services:\n  redis:\n    image: " + tc.image + "\n")); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
