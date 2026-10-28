package commands

// update_plan_hasura_test.go — the hasura row of `nself update --plan` follows
// compose.HasuraEntry (P7-LIVE-23): v1.4 stays on v2.44.0, v1.5 offers v2.51.0
// with a one-way note, and a pin newer than the offer is never moved back.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
)

func TestUpdatePlanHasuraRow(t *testing.T) {
	v14, ok1 := compose.LockedRef("hasura-v14")
	cur, ok2 := compose.LockedRef("hasura")
	if !ok1 || !ok2 {
		t.Fatal("image lock needs hasura and hasura-v14")
	}
	older := "hasura/graphql-engine:v2.43.0"
	newer := "docker.io/hasura/graphql-engine:v2.99.0@sha256:" + strings.Repeat("ab", 32)
	for _, tc := range []struct {
		name       string
		v15        bool
		pin        string
		want, note string // want "" = no value; the reason must then contain the note text
	}{
		{"fork is not upstream", false, "ghcr.io/acme/hasura-fork:v2.60.0", "", "repository"},
		{"migration suffix", false, "hasura/graphql-engine:v2.44.0.cli-migrations-v3", "", "tag"},
		{"ce suffix", true, "hasura/graphql-engine:v2.51.0-ce", "", "tag"},
		{"latest tag", false, "hasura/graphql-engine:latest", "", "tag"},
		{"v1.4 keeps v2.44.0", false, v14.String(), v14.String(), ""},
		{"v1.4 moves an older pin to v2.44.0 without a note", false, older, v14.String(), ""},
		{"v1.4 never moves a v2.51.0 pin back", false, cur.String(), "", "backup restored"},
		{"v1.4 never moves a digest-only v2.51.0 pin back", false, "docker.io/hasura/graphql-engine@" + cur.IndexDigest, "", "backup restored"},
		{"v1.5 moves v2.44.0 to v2.51.0 with a one-way note", true, v14.String(), cur.String(), "one-way"},
		{"v1.5 moves an older tag with a note", true, older, cur.String(), "one-way"},
		{"v1.5 keeps v2.51.0 without a note", true, cur.String(), cur.String(), ""},
		{"v1.5 never moves a newer pin back", true, newer, "", "backup restored"},
		{"v1.5 digest-only v2.44.0 pin moves with a one-way note", true, "docker.io/hasura/graphql-engine@" + v14.IndexDigest, cur.String(), "one-way"},
		{"v1.4 digest-only v2.44.0 pin stays", false, "docker.io/hasura/graphql-engine@" + v14.IndexDigest, v14.String(), ""},
		{"v1.5 unknown digest-only pin is left as is", true, "docker.io/hasura/graphql-engine@sha256:" + strings.Repeat("cd", 32), "", "does not know"},
		{"v1.4 trusts the v2.51.0 digest over a v2.44.0 tag", false, "docker.io/hasura/graphql-engine:" + v14.Version + "@" + cur.IndexDigest, "", "backup restored"},
		{"v1.5 trusts the v2.44.0 digest over a v2.51.0 tag", true, "docker.io/hasura/graphql-engine:" + cur.Version + "@" + v14.IndexDigest, cur.String(), "one-way"},
		{"v1.5 trusts the tag when the digest is unknown", true, newer, "", "backup restored"},
		{"v1.4 unknown digest-only pin is left as is", false, "docker.io/hasura/graphql-engine@sha256:" + strings.Repeat("cd", 32), "", "does not know"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			c := hasuraCandidate(tc.pin)
			if tc.want == "" {
				if c.Value != "" || !strings.Contains(c.Reason, tc.note) {
					t.Fatalf("candidate = %+v, want no value and a reason containing %q", c, tc.note)
				}
				return
			}
			if c.Value != tc.want || c.Reason != "" {
				t.Fatalf("candidate = %+v, want value %q", c, tc.want)
			}
			if tc.note == "" && c.Note != "" || tc.note != "" && !strings.Contains(c.Note, tc.note) {
				t.Errorf("note = %q, want it to contain %q", c.Note, tc.note)
			}
		})
	}
}
