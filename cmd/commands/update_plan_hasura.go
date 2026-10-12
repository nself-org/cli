package commands

// Purpose: the hasura row of `nself update` (P7-LIVE-23). The hasura service has
// two image lock entries on hasura/graphql-engine (hasura-v14 = v2.44.0, hasura =
// v2.51.0, which migrates the metadata catalog one way), so the generic
// per-repository lookup cannot choose between them.
// Inputs: the project's hasura pin.
// Outputs: the plan candidate for the hasura row.
// Constraints: the entry is compose.HasuraEntry() (v1.4 stays on v2.44.0); a pin
// newer than the entry is never moved back (that needs the pre-upgrade backup
// restored); the move to v2.51.0 carries a one-way note.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/upgrade/plan"
)

// hasuraCandidate is the hasura row: the entry compose.HasuraEntry picks for the mode.
func hasuraCandidate(pin string) plan.Candidate {
	r, ok := compose.LockedRef(compose.HasuraEntry())
	cur, _ := compose.LockedRef("hasura")
	if !ok {
		return plan.Candidate{Reason: plan.ReasonNoImage}
	}
	switch cmp := hasuraPinOrder(pin, r, cur); {
	case cmp > 0:
		return plan.Candidate{Reason: fmt.Sprintf("the pinned Hasura is newer than the %s this mode offers and has migrated the metadata catalog one way; going back needs the pre-upgrade backup restored (Image-Lock wiki, Roll back)", r.Version)}
	case cmp < 0 && r.IndexDigest == cur.IndexDigest:
		return plan.Candidate{Value: r.String(), Note: "Hasura " + r.Version + " migrates the metadata catalog one way on first start: take a verified backup first"}
	}
	return plan.Candidate{Value: r.String()}
}

// hasuraPinOrder compares a hasura pin with the candidate entry: >0 when the pin is a
// newer release, <0 when older, 0 when equal or when the lock cannot tell (a digest the
// lock does not know). The digest of the current lock entry is a release the lock names.
func hasuraPinOrder(pin string, cand, cur compose.Ref) int {
	base, digest, _ := strings.Cut(pin, "@")
	if i := strings.LastIndex(base, ":"); i > strings.LastIndex(base, "/") {
		if c, ok := compareHasuraVersion(base[i+1:], cand.Version); ok {
			return c
		}
	}
	if digest != "" && digest == cur.IndexDigest {
		c, _ := compareHasuraVersion(cur.Version, cand.Version)
		return c
	}
	return 0
}

// compareHasuraVersion orders two vMAJOR.MINOR.PATCH tags; ok is false for anything else.
func compareHasuraVersion(a, b string) (int, bool) {
	var x, y [3]int
	if _, err := fmt.Sscanf(a, "v%d.%d.%d", &x[0], &x[1], &x[2]); err != nil {
		return 0, false
	}
	if _, err := fmt.Sscanf(b, "v%d.%d.%d", &y[0], &y[1], &y[2]); err != nil {
		return 0, false
	}
	for i := range x {
		if x[i] != y[i] {
			if x[i] > y[i] {
				return 1, true
			}
			return -1, true
		}
	}
	return 0, true
}
