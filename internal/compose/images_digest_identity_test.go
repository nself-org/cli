package compose

import (
	"strings"
	"testing"
)

func TestResolveImage_DigestIdentity(t *testing.T) {
	old := ImageDigests
	t.Cleanup(func() { ImageDigests = old })
	ImageDigests = map[string]string{}
	for i, name := range []string{"auth", "auth-v14", "redis", "valkey"} {
		entry, ok := LockedRef(name)
		if !ok {
			t.Fatalf("missing lock entry %s", name)
		}
		ImageDigests[entry.LegacyRef] = strings.Repeat(string(rune('a'+i)), 64)
	}
	// Old service keys cannot identify which image was inspected. Ignore them.
	ImageDigests["auth"] = strings.Repeat("e", 64)
	ImageDigests["redis"] = strings.Repeat("f", 64)
	for _, tc := range []struct{ service, entry string }{
		{"auth", "auth"}, {"auth", "auth-v14"},
		{"redis", "redis"}, {"redis", "valkey"},
	} {
		t.Run(tc.entry, func(t *testing.T) {
			entry, _ := LockedRef(tc.entry)
			want := entry.LegacyRef + "@sha256:" + ImageDigests[entry.LegacyRef]
			if got := ResolveImage(tc.service, entry.LegacyRef); got != want {
				t.Errorf("ResolveImage = %q, want %q", got, want)
			}
		})
	}
	for _, service := range []string{"auth", "redis"} {
		image := "custom/" + service + ":different"
		if got := ResolveImage(service, image); got != image {
			t.Errorf("uninspected image = %q, want %q", got, image)
		}
	}
}
