package lock

import "testing"

// TestPinLocalDigestSpelling keeps matching CLI pins stable across Docker caches
// while preserving a different locally resolved digest.
func TestPinLocalDigestSpelling(t *testing.T) {
	const (
		ref       = "hasura/graphql-engine:v2.44.0"
		digest    = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
		other     = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
		canonical = "docker.io/" + ref + "@" + digest
	)
	for _, tc := range []struct {
		name   string
		locked string
		found  bool
		want   string
	}{
		{"matching CLI digest", canonical, true, canonical},
		{"different CLI digest", "docker.io/" + ref + "@" + other, true, ref + "@" + digest},
		{"malformed CLI pin", "docker.io/" + ref + "@@" + digest, true, ref + "@" + digest},
		{"missing CLI pin", canonical, false, ref + "@" + digest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Resolver{
				LocalDigest: func(string) string { return digest },
				CLILocked:   func(string) (string, bool) { return tc.locked, tc.found },
			}
			images, unpinned := Pin(map[string]string{"hasura": ref}, r)
			if images["hasura"] != tc.want || len(unpinned) != 0 {
				t.Fatalf("Pin = %v, unpinned %v; want hasura %q", images, unpinned, tc.want)
			}
		})
	}
}
