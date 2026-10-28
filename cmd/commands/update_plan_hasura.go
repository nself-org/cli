package commands

// Purpose: the hasura row of `nself update` (P7-LIVE-23). The hasura service has
// two image lock entries on hasura/graphql-engine (hasura-v14 = v2.44.0, hasura =
// v2.51.0, treated as a one-way move), so the generic
// per-repository lookup cannot choose between them.
// Inputs: the project's hasura pin.
// Outputs: the plan candidate for the hasura row.
// Constraints: the entry is compose.HasuraEntry() (v1.4 stays on v2.44.0); a pin
// newer than the entry is never moved back (that needs the pre-upgrade backup
// restored); the move to v2.51.0 carries a one-way note (restore the backup to go back).

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/upgrade/plan"
)

// hasuraCandidate is the hasura row: the entry compose.HasuraEntry picks for the mode.
func hasuraCandidate(pin string) plan.Candidate {
	r, ok := compose.LockedRef(compose.HasuraEntry())
	v14, _ := compose.LockedRef("hasura-v14")
	cur, _ := compose.LockedRef("hasura")
	if !ok {
		return plan.Candidate{Reason: plan.ReasonNoImage}
	}
	base, _, _ := strings.Cut(pin, "@")
	repo := base
	tag := ""
	if i := strings.LastIndex(base, ":"); i > strings.LastIndex(base, "/") {
		repo, tag = base[:i], base[i+1:]
	}
	if repo != "hasura/graphql-engine" && repo != "docker.io/hasura/graphql-engine" {
		return plan.Candidate{Reason: "the Hasura pin uses a different repository; it is left as is"}
	}
	if tag != "" && !plainHasuraTag.MatchString(tag) {
		return plan.Candidate{Reason: "the Hasura tag is not a plain vX.Y.Z release tag; update cannot replace a variant or moving tag with the upstream image; it is left as is"}
	}
	cmp, known := hasuraPinOrder(pin, r, v14, cur)
	switch {
	case !known:
		return plan.Candidate{Reason: "the Hasura pin names a digest the CLI image lock does not know, so update cannot tell which release it is; it is left as is"}
	case cmp > 0:
		return plan.Candidate{Reason: fmt.Sprintf("the pinned Hasura is newer than the %s this mode offers; the pin-back is treated as one way, so going back needs the pre-upgrade backup restored (Image-Lock wiki, Roll back)", r.Version)}
	case cmp < 0 && r.IndexDigest == cur.IndexDigest:
		return plan.Candidate{Value: r.String(), Note: "Hasura " + r.Version + " is treated as a one-way move: take a verified backup first, and restore it to go back (the metadata catalog version is the same on v2.44.0 and v2.51.0, but pinning back alone is unsupported)"}
	}
	return plan.Candidate{Value: r.String()}
}

// hasuraPinOrder compares a hasura pin with the candidate entry: >0 when the pin is a
// newer release, <0 when older, 0 when equal. known is false for a pin without a
// parseable tag whose digest is neither lock entry's (the lock cannot name the release).
// A pin's digest is what the daemon runs, so a digest one of the entries owns
// orders as that entry's release, whatever its tag says (a hand-edited
// tag@digest pin that disagrees must not defeat the downgrade guard, D-1154);
// the tag decides only when the digest is absent or not in the lock.
func hasuraPinOrder(pin string, cand, v14, cur compose.Ref) (order int, known bool) {
	base, digest, _ := strings.Cut(pin, "@")
	repo := base
	if i := strings.LastIndex(base, ":"); i > strings.LastIndex(base, "/") {
		repo = base[:i]
		if !plainHasuraTag.MatchString(base[i+1:]) {
			return 0, false
		}
	}
	if repo != "hasura/graphql-engine" && repo != "docker.io/hasura/graphql-engine" {
		return 0, false
	}
	for _, e := range []compose.Ref{v14, cur} {
		if digest != "" && digest == e.IndexDigest {
			return compareHasuraVersion(e.Version, cand.Version)
		}
	}
	if i := strings.LastIndex(base, ":"); i > strings.LastIndex(base, "/") {
		if c, ok := compareHasuraVersion(base[i+1:], cand.Version); ok {
			return c, true
		}
	}
	return 0, false
}

var plainHasuraTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// compareHasuraVersion orders two vMAJOR.MINOR.PATCH tags; ok is false for anything else.
func compareHasuraVersion(a, b string) (int, bool) {
	if !plainHasuraTag.MatchString(a) || !plainHasuraTag.MatchString(b) {
		return 0, false
	}
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
