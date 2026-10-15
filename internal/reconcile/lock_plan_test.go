package reconcile

// lock_plan_test.go — plan == apply for nself.lock.yaml (P7-LIVE-09, review F10).
//
// Purpose: the lock is a planned artifact of kind "lock": a plan lists it when it
// would change, writes nothing, and the apply writes exactly the planned bytes
// (the held render would fail with E452 otherwise).
// Inputs: the dev-minimal fixture in v1.5 mode (the lock is always on).
// Outputs: assertions only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/lock"
)

func lockArtifact(p *Plan) (Artifact, bool) {
	for _, a := range p.Artifacts {
		if a.Path == lock.FileName {
			return a, true
		}
	}
	return Artifact{}, false
}

// TestLockPlanApply: a lock that does not exist yet (add) and a lock whose
// content changes (change) are both planned exactly as applied, and the plan
// after the apply is empty.
func TestLockPlanApply(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	compattest.Set(t, true)
	path := filepath.Join(f.project, lock.FileName)

	// 1. No lock yet: the plan adds it and writes nothing.
	p := planOf(t, f, nil)
	a, ok := lockArtifact(p)
	if !ok || a.Kind != KindLock || a.Action != ActionAdd {
		t.Fatalf("plan must add %s as kind lock, got %+v (listed %v)", lock.FileName, a, ok)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("plan wrote the lock (stat err %v)", err)
	}
	applyFixture(t, f)
	if again := planOf(t, f, nil); !settledPlan(again) {
		t.Fatalf("plan after the first apply is not empty: %+v", again.Artifacts)
	}

	// 2. The lock's content changes (an explicit HASURA_VERSION moves the compose
	// and the lock): the plan changes exactly the files the apply changes, the
	// lock among them, and the apply writes the planned bytes.
	before := fileHashes(t, f.project)
	writeFile(t, filepath.Join(f.project, ".env"), readFile(t, filepath.Join(f.project, ".env"))+"HASURA_VERSION=v2.41.0\n", 0o600)
	changedEnv := fileHashes(t, f.project)[".env"]
	p = planOf(t, f, nil)
	if a, ok := lockArtifact(p); !ok || a.Action != ActionChange {
		t.Fatalf("a changed lock must be planned as change, got %+v (listed %v)", a, ok)
	}
	if after := fileHashes(t, f.project); after[lock.FileName] != before[lock.FileName] || len(after) != len(before) {
		t.Fatal("plan changed the project")
	}
	applyFixture(t, f)
	after := fileHashes(t, f.project)
	if after[lock.FileName] == before[lock.FileName] {
		t.Fatal("apply did not change the lock")
	}
	changed := map[string]bool{}
	for k, v := range after {
		if v != before[k] && !(k == ".env" && v == changedEnv) && k != ".nself/op.lock" {
			changed[k] = true
		}
	}
	planned := map[string]bool{}
	for _, a := range p.Artifacts {
		planned[a.Path] = true
	}
	for k := range changed {
		if !planned[k] && !strings.HasPrefix(k, ".nself/backups/") {
			t.Errorf("apply changed %s, which the plan did not list", k)
		}
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "v2.41.0") || strings.Contains(string(got), "v2.44.0@") {
		t.Fatalf("the lock does not record the new hasura version:\n%s", got)
	}
	if again := planOf(t, f, nil); !settledPlan(again) {
		t.Fatalf("plan after the second apply is not empty: %+v", again.Artifacts)
	}
}
