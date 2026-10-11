package promote

import (
	"context"
	"testing"
	"time"
)

type restoreRemote struct{ commands []string }

func (r *restoreRemote) Label() string { return "target-app" }
func (r *restoreRemote) Run(_ context.Context, command string) ([]byte, error) {
	r.commands = append(r.commands, command)
	return nil, nil
}
func (*restoreRemote) Rsync(context.Context, []string, string, string, string) error { return nil }

func TestSnapshotUndoRecordsRestore(t *testing.T) {
	r := &restoreRemote{}
	st := &State{BackupID: "backup-fixture"}
	step := SnapshotStep(r, "/srv/fixture", "pre-promote-1-aaaaaaaa")
	if err := step.Undo(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 1 || st.DBRestoredAt == "" {
		t.Fatalf("restore commands=%v restored_at=%q", r.commands, st.DBRestoredAt)
	}
	if _, err := time.Parse(time.RFC3339Nano, st.DBRestoredAt); err != nil {
		t.Fatalf("DBRestoredAt %q: %v", st.DBRestoredAt, err)
	}
}
