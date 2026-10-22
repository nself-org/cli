package compose

import "testing"

func TestCacheImageExpanded(t *testing.T) {
	t.Setenv("CACHE_IMAGE", "redis:7-alpine")
	t.Setenv("EMPTY_CACHE_IMAGE", "")
	for _, tc := range []struct{ in, want string }{
		{"$CACHE_IMAGE", "redis:7-alpine"},
		{"${CACHE_IMAGE:-valkey/valkey:8}", "redis:7-alpine"},
		{"${EMPTY_CACHE_IMAGE:-valkey/valkey:8}", "valkey/valkey:8"},
		{"${EMPTY_CACHE_IMAGE-valkey/valkey:8}", ""},
		{"${EMPTY_CACHE_IMAGE:-${CACHE_IMAGE}}", "redis:7-alpine"},
		{"${CACHE_IMAGE:+valkey/valkey:8}", "valkey/valkey:8"},
		{"${EMPTY_CACHE_IMAGE:+valkey/valkey:8}", ""},
		{"${EMPTY_CACHE_IMAGE:?required}", ""},
		{"$$CACHE_IMAGE", "$CACHE_IMAGE"},
		{"${CACHE_IMAGE", ""},
	} {
		if got := cacheImageExpanded(tc.in); got != tc.want {
			t.Errorf("expand %q = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCacheEngineMountedLongVolume(t *testing.T) {
	in := []byte("services:\n  redis:\n    image: custom/cache:1\n    volumes:\n      - type: volume\n        source: valkey_data\n        target: /data\nvolumes:\n  redis_data: {}\n")
	if got := ExistingCacheEngine(in); got != EngineValkey {
		t.Errorf("got %s", got)
	}
}
