package reconcile

// sites_effect.go — when a plan carries the nginx/sites snapshot (D-0757).
//
// Purpose: the snapshot under .nself/backups/nginx-sites-<ts>/ protects site confs a build rewrites
// or removes. The plan lists it only then, and apply must take it only then, or a plan that reports
// nothing (or no snapshot) would still be followed by a write into .nself/backups.
// Inputs: the planned build and the plan's artifacts. Outputs: the planned build the write is held
// to, without the snapshot effect when the plan lists none.
// Constraints: the build sink treats a render with no nginx-sites-backup effect as "take no snapshot".

import (
	"slices"
	"strings"

	nbuild "github.com/nself-org/cli/internal/build"
)

// hasSitesChange reports whether the plan changes or removes a file under an
// nginx/sites directory (the project's or the fronting stack's).
func hasSitesChange(arts []Artifact) bool {
	for _, a := range arts {
		if a.Kind == KindNginx && (a.Action != ActionAdd) && strings.Contains(a.Path, "nginx/sites/") {
			return true
		}
	}
	return false
}

// holdSnapshotToPlan drops the nginx-sites-backup effect from pb when the plan changes no site conf.
func holdSnapshotToPlan(pb *nbuild.PlannedBuild, arts []Artifact) {
	if hasSitesChange(arts) {
		return
	}
	pb.Effects = slices.DeleteFunc(slices.Clone(pb.Effects), func(e nbuild.PlannedEffect) bool {
		return e.Kind == nbuild.EffectNginxSitesBackup
	})
}
