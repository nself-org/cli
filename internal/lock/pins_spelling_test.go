package lock

import (
	"strings"
	"testing"
)

func TestPinLocalDigestSpelling(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	other := "sha256:" + strings.Repeat("b", 64)
	ref := "hasura/graphql-engine:v2.44.0"
	locked := "docker.io/" + ref + "@" + digest
	for _, local := range []string{digest, other} {
		images, _ := Pin(map[string]string{"hasura": ref}, Resolver{
			LocalDigest: func(string) string { return local },
			CLILocked:   func(string) (string, bool) { return locked, true },
		})
		want := locked
		if local != digest {
			want = ref + "@" + local
		}
		if images["hasura"] != want {
			t.Fatalf("got %s, want %s", images["hasura"], want)
		}
	}
}
