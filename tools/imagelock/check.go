package main

// check.go — the offline half: `imagelock -check`.
//
// Purpose: prove the committed lock is well formed and covers images.yaml
// without touching the network, so CI and `go test` can gate on it.
// Inputs: the lock bytes and the authored sources.
// Outputs: a list of human-readable problems (empty means pass).
// Constraints: lock ⊇ sources; an entry whose repository, version, role,
// legacy_ref or required platforms drifted from images.yaml is stale. Lock
// entries with no source (merged from images.json) are allowed.

import (
	"fmt"

	"github.com/nself-org/cli/internal/compose"
)

// Check returns every problem found; nil means the lock is current.
func Check(lockData []byte, sources []Source) []string {
	lf, err := compose.ParseLock(lockData)
	if err != nil {
		return []string{err.Error()}
	}
	by := map[string]compose.Ref{}
	for _, r := range lf.Images {
		by[r.Name] = r
	}
	var problems []string
	for _, s := range sources {
		r, ok := by[s.Name]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: in images.yaml but not in the lock; run `go run ./tools/imagelock -resolve`", s.Name))
			continue
		}
		if r.Repository != s.Repository || r.Version != s.Version || r.Role != s.Role || r.LegacyRef != s.LegacyRef {
			problems = append(problems, fmt.Sprintf("%s: lock entry differs from images.yaml (repository/version/role/legacy_ref); run -resolve", s.Name))
		}
		for _, p := range s.Platforms {
			if r.Platforms[p] == "" {
				problems = append(problems, fmt.Sprintf("%s: lock has no %s digest", s.Name, p))
			}
		}
	}
	return problems
}
