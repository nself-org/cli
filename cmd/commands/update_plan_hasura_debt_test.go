package commands

import (
	"github.com/nself-org/cli/internal/compose"
	"testing"
)

func TestHasuraPinOrderRejectsAmbiguousTags(t *testing.T) {
	cand, _ := compose.LockedRef("hasura")
	old, _ := compose.LockedRef("hasura-v14")
	for _, tc := range []struct {
		pin   string
		known bool
	}{
		{"hasura/graphql-engine:v2.44.0", true},
		{"docker.io/hasura/graphql-engine:v2.44.0", true},
		{"example.com/hasura/graphql-engine:v2.44.0", false},
		{"fork/graphql-engine:v2.44.0", false},
		{"hasura/graphql-engine:v2.44.0-ce", false},
		{"hasura/graphql-engine:v2.44.0.1", false},
		{"hasura/graphql-engine@" + old.IndexDigest, true},
		{"fork/graphql-engine@" + old.IndexDigest, false},
	} {
		t.Run(tc.pin, func(t *testing.T) {
			_, known := hasuraPinOrder(tc.pin, cand, old, cand)
			if known != tc.known {
				t.Fatalf("known=%v, want %v", known, tc.known)
			}
		})
	}
}
