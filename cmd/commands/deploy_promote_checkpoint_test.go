package commands

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
)

func TestPromoteProvisionalCheckpoints(t *testing.T) {
	root := t.TempDir()
	r := &promoteSafetyRemote{label: "prod-app"}
	w := &promoteWiring{workdir: root, targetRemote: r, targetServer: controlplane.Server{Host: "example.invalid", RemotePath: "/srv/fixture"}, started: time.Now()}
	st := &cpromote.State{Source: "ls", Target: "prodsim", Values: map[string]any{}}
	steps := promoteSteps(w, false)
	if err := steps[8].Run(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	rec, err := cpromote.LoadRecord(root)
	if err != nil || rec == nil || rec.Status != "in_progress" || rec.BackupID == "" || rec.BackupHost != r.Label() {
		t.Fatalf("snapshot checkpoint = %+v, %v", rec, err)
	}
	st.MigrateStarted = true
	if err := steps[11].Run(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	rec, err = cpromote.LoadRecord(root)
	if err != nil || !rec.MigrateStarted {
		t.Fatalf("migrate checkpoint = %+v, %v", rec, err)
	}
}
