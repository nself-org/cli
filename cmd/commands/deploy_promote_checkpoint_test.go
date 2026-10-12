package commands

import (
	"context"
	"testing"

	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
)

func TestPromoteProvisionalCheckpoints(t *testing.T) {
	w, st, r := promoteProofFixture(t)
	root := w.workdir
	st.MigrateStarted = false
	steps := promoteSteps(w, false)
	if err := steps[8].Run(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	rec, err := cpromote.LoadAttempt(root)
	if err != nil || rec == nil || rec.Status != "in_progress" || rec.BackupID == "" || rec.BackupHost != r.Label() {
		t.Fatalf("snapshot checkpoint = %+v, %v", rec, err)
	}
	st.MigrateStarted = true
	if err := steps[11].Run(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	rec, err = cpromote.LoadAttempt(root)
	if err != nil || !rec.MigrateStarted {
		t.Fatalf("migrate checkpoint = %+v, %v", rec, err)
	}
}
