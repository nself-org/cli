package reconcile

import (
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
)

// TestBackupEffectWithoutArtifactChanges pins backup-only plans: an apply still
// writes a snapshot when the rendered site files already match the disk.
func TestBackupEffectWithoutArtifactChanges(t *testing.T) {
	p := Plan{Env: "dev", Effects: effectsOf([]nbuild.PlannedEffect{{Kind: nbuild.EffectNginxSitesBackup, Target: ".nself/backups"}}), Containers: Containers{Known: true}}
	if err := p.Finalize(); err != nil {
		t.Fatal(err)
	}
	if len(p.Effects) != 1 || p.Effects[0].Kind != EffectNginxSitesBkp || p.Empty {
		t.Fatalf("backup-only plan must retain its effect and be nonempty: %+v", p)
	}
}

// settledPlan permits only the snapshot a repeat build takes. It still rejects
// artifact drift, container changes, unknown state and every other effect.
func settledPlan(p *Plan) bool {
	if len(p.Artifacts) != 0 || !p.Containers.Known || len(p.Containers.Items) != 0 {
		return false
	}
	for _, e := range p.Effects {
		if e.Kind != EffectNginxSitesBkp {
			return false
		}
	}
	return true
}

func TestBuiltProjectPlansSnapshotWithoutArtifactChanges(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	applyFixture(t, f)
	p := planOf(t, f, nil)
	if !settledPlan(p) || len(p.Effects) != 1 || p.Empty {
		t.Fatalf("repeat build must list only its snapshot: %+v", p)
	}
}
