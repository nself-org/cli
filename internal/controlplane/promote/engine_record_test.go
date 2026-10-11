package promote

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPromoteRecordFailureLeavesPromotionLive(t *testing.T) {
	cause := errors.New("record disk is full")
	var undos []string
	steps := canonicalSteps(func(name string, _ *State) error {
		if name == "record" {
			return cause
		}
		return nil
	}, func(name string, _ *State) error {
		undos = append(undos, name)
		return nil
	})
	st := &State{SnapshotTag: "pre-promote-4-aaaaaaaa", BackupID: "backup-4",
		PreDeployTar: map[string]string{"worker-b": "/srv/b/backups/pre-deploy-b.tar", "worker-a": "/srv/a/backups/pre-deploy-a.tar"}}
	result, err := (Engine{Steps: steps}).Run(context.Background(), st)
	if result.Status != "failed" || result.FailedStep != "record" || len(undos) != 0 {
		t.Fatalf("record failure undid a live promotion: result=%#v undos=%v err=%v", result, undos, err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("record failure lost cause: %v", err)
	}
	for _, want := range []string{"E489", "promotion is live", st.SnapshotTag, st.BackupID, st.PreDeployTar["worker-a"], st.PreDeployTar["worker-b"], "by hand"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("record failure lacks %q: %v", want, err)
		}
	}
}

func TestPromoteCancelDuringMigrateCompensates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var undos []string
	steps := canonicalSteps(func(string, *State) error { return nil }, nil)
	steps[indexOf("migrate")].Run = func(stepCtx context.Context, st *State) error {
		if !st.MigrateStarted {
			t.Fatal("migration started without setting rollback evidence")
		}
		cancel()
		<-stepCtx.Done()
		return stepCtx.Err()
	}
	for _, name := range []string{"apply", "snapshot"} {
		name := name
		steps[indexOf(name)].Undo = func(rollbackCtx context.Context, _ *State) error {
			if rollbackCtx.Err() != nil {
				t.Fatalf("%s compensation received cancelled context", name)
			}
			undos = append(undos, name)
			return nil
		}
	}
	result, err := (Engine{Steps: steps}).Run(ctx, &State{})
	if result.Status != "rolled_back" || result.FailedStep != "migrate" || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "E488") {
		t.Fatalf("migration cancellation = %#v, %v", result, err)
	}
	if strings.Join(undos, ",") != "apply,snapshot" {
		t.Fatalf("migration cancellation compensations = %v", undos)
	}
}
