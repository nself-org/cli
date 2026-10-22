package commands

import (
	"github.com/nself-org/cli/internal/compose"
	"testing"
)

func TestHasuraDigestPrecedesTag(t *testing.T) {
	old, _ := compose.LockedRef("hasura-v14")
	cur, _ := compose.LockedRef("hasura")
	for _, tc := range []struct {
		pin   string
		order int
		known bool
	}{
		{"hasura/graphql-engine:v2.44.0@" + cur.IndexDigest, 1, true},
		{"hasura/graphql-engine:v2.51.0@" + old.IndexDigest, 0, true},
		{"hasura/graphql-engine:v2.44.0@sha256:unknown", 0, false},
	} {
		got, known := hasuraPinOrder(tc.pin, old, old, cur)
		if got != tc.order || known != tc.known {
			t.Errorf("%s: got (%d,%v), want (%d,%v)", tc.pin, got, known, tc.order, tc.known)
		}
	}
}
