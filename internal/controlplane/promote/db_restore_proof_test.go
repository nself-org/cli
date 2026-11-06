package promote

// Purpose: pin C2-01 on the promote side: the ssh restore carries
// --confirm-project, and a restore without a project name is refused before
// the intent checkpoint and before any remote command.

import (
	"context"
	"strings"
	"testing"
)

func TestRestoreDatabasePassesConfirmProject(t *testing.T) {
	r := &restoreRemote{}
	var phase, restored string
	if err := RestoreDatabase(context.Background(), r, "/srv/app", "demo", "pre-promote-1-aaaaaaaa", "backup-1", &phase, &restored, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 1 || !strings.Contains(r.commands[0], "nself backup restore 'backup-1' --yes --confirm-project 'demo'") {
		t.Fatalf("commands %v", r.commands)
	}
}

func TestRestoreDatabaseWithoutProjectChangesNothing(t *testing.T) {
	r := &restoreRemote{}
	var phase, restored string
	saves := 0
	err := RestoreDatabase(context.Background(), r, "/srv/app", "", "pre-promote-1-aaaaaaaa", "backup-1", &phase, &restored, func() error { saves++; return nil })
	if err == nil || !strings.Contains(err.Error(), "--confirm-project") {
		t.Fatalf("want a refusal naming --confirm-project, got %v", err)
	}
	if len(r.commands) != 0 || saves != 0 || phase != "" {
		t.Fatalf("refusal had effects: commands=%v saves=%d phase=%q", r.commands, saves, phase)
	}
}
