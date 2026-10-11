package promote

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingObserver struct{ events []string }

func (o *recordingObserver) Start(s string)                 { o.events = append(o.events, "start:"+s) }
func (o *recordingObserver) Done(s string, _ time.Duration) { o.events = append(o.events, "done:"+s) }
func (o *recordingObserver) Failed(s string, _ error)       { o.events = append(o.events, "failed:"+s) }
func (o *recordingObserver) Skipped(s, why string)          { o.events = append(o.events, "skipped:"+s+":"+why) }
func (o *recordingObserver) Rollback(s string, err error) {
	status := "ok"
	if err != nil {
		status = "failed"
	}
	o.events = append(o.events, "rollback:"+s+":"+status)
}

func canonicalSteps(run func(string, *State) error, undo func(string, *State) error) []Step {
	steps := make([]Step, len(StepNames))
	for i, name := range StepNames {
		name := name
		steps[i] = Step{Name: name, Run: func(_ context.Context, st *State) error { return run(name, st) }}
		if undo != nil {
			steps[i].Undo = func(_ context.Context, st *State) error { return undo(name, st) }
		}
	}
	return steps
}

func TestPromoteEngineOrder(t *testing.T) {
	var ran []string
	observer := &recordingObserver{}
	engine := Engine{Steps: canonicalSteps(func(name string, _ *State) error {
		ran = append(ran, name)
		return nil
	}, nil), Observer: observer, Now: func() time.Time { return time.Unix(1, 0) }}
	result, err := engine.Run(context.Background(), &State{Source: "ls", Target: "prodsim"})
	if err != nil || result.Status != "ok" {
		t.Fatalf("Run() = status %q, err %v", result.Status, err)
	}
	wantRan := strings.Join(append(append([]string{}, StepNames[:2]...), StepNames[3:]...), ",")
	if strings.Join(ran, ",") != wantRan {
		t.Fatalf("run order = %v, want %s", ran, wantRan)
	}
	if !contains(observer.events, "skipped:verify:no release manifest") {
		t.Fatalf("observer events lack verify skip: %v", observer.events)
	}
	var wantEvents []string
	for _, name := range StepNames {
		if name == "verify" {
			wantEvents = append(wantEvents, "skipped:verify:no release manifest")
			continue
		}
		wantEvents = append(wantEvents, "start:"+name, "done:"+name)
	}
	if strings.Join(observer.events, ",") != strings.Join(wantEvents, ",") {
		t.Fatalf("observer events = %v, want %v", observer.events, wantEvents)
	}
	verifyCalls := 0
	withVerify := Engine{Steps: canonicalSteps(func(name string, _ *State) error {
		if name == "verify" {
			t.Fatal("verify step Run called instead of Verify hook")
		}
		return nil
	}, nil), Verify: func(context.Context, *State) error { verifyCalls++; return nil }}
	if _, err = withVerify.Run(context.Background(), &State{}); err != nil || verifyCalls != 1 {
		t.Fatalf("Verify hook calls=%d, err=%v", verifyCalls, err)
	}
	bad := engine
	bad.Steps = append([]Step(nil), engine.Steps...)
	bad.Steps[0], bad.Steps[1] = bad.Steps[1], bad.Steps[0]
	if _, err = bad.Run(context.Background(), &State{}); err == nil || !strings.Contains(err.Error(), "step 0") {
		t.Fatalf("out-of-order Run error = %v", err)
	}
}

func TestPromoteRollback(t *testing.T) {
	var events []string
	healthCalls := 0
	ctx, cancel := context.WithCancel(context.Background())
	steps := canonicalSteps(func(name string, _ *State) error {
		if name == "health" {
			healthCalls++
			if healthCalls == 1 {
				cancel()
				return errors.New("unhealthy")
			}
			events = append(events, "health")
		}
		return nil
	}, func(name string, _ *State) error {
		events = append(events, "undo:"+name)
		return nil
	})
	for i := range steps {
		if steps[i].Undo == nil {
			continue
		}
		name := steps[i].Name
		steps[i].Undo = func(rollbackCtx context.Context, _ *State) error {
			if rollbackCtx.Err() != nil {
				t.Fatalf("rollback %s inherited cancellation: %v", name, rollbackCtx.Err())
			}
			events = append(events, "undo:"+name)
			return nil
		}
	}
	result, err := (Engine{Steps: steps}).Run(ctx, &State{SnapshotTag: "pre-promote-1-aaaaaaaa", BackupID: "backup-1"})
	if result.Status != "rolled_back" || err == nil || !strings.Contains(err.Error(), "E488") || !strings.Contains(err.Error(), "health") {
		t.Fatalf("health failure = %#v, %v", result, err)
	}
	if strings.Join(events, ",") != "undo:health,undo:migrate,undo:apply,undo:save,undo:snapshot,health" || healthCalls != 2 {
		t.Fatalf("rollback events=%v health calls=%d", events, healthCalls)
	}

	events = nil
	steps = canonicalSteps(func(name string, _ *State) error {
		if name == "apply" {
			return errors.New("apply")
		}
		return nil
	}, func(name string, _ *State) error { events = append(events, "undo:"+name); return nil })
	result, err = (Engine{Steps: steps}).Run(context.Background(), &State{})
	if result.Status != "rolled_back" || err == nil || contains(events, "undo:snapshot") {
		t.Fatalf("apply failure = %#v, %v, events %v", result, err, events)
	}

	steps = canonicalSteps(func(name string, _ *State) error {
		if name == "snapshot" {
			return errors.New("verify backup")
		}
		return nil
	}, func(name string, _ *State) error { t.Fatalf("undo ran after snapshot failure: %s", name); return nil })
	result, err = (Engine{Steps: steps}).Run(context.Background(), &State{})
	if result.Status != "failed" || err == nil || !strings.Contains(err.Error(), "verify backup") {
		t.Fatalf("snapshot failure = %#v, %v", result, err)
	}
}

func TestPromoteRollbackCauseAndHealth(t *testing.T) {
	cause := errors.New("migration refused")
	var events []string
	steps := canonicalSteps(func(name string, _ *State) error {
		if name == "migrate" {
			return cause
		}
		if name == "health" {
			events = append(events, "health")
		}
		return nil
	}, func(name string, _ *State) error { events = append(events, "undo:"+name); return nil })
	result, err := (Engine{Steps: steps}).Run(context.Background(), &State{})
	if result.Status != "rolled_back" || !errors.Is(err, cause) {
		t.Fatalf("trigger cause lost: %#v %v", result, err)
	}
	if strings.Join(events, ",") != "undo:migrate,undo:apply,undo:save,undo:snapshot,health" {
		t.Fatalf("migrate rollback order %v", events)
	}
	steps[indexOf("health")].Run = func(context.Context, *State) error { return errors.New("rollback unhealthy") }
	result, err = (Engine{Steps: steps}).Run(context.Background(), &State{SnapshotTag: "pre-promote-1-aaaaaaaa", BackupID: "backup-1"})
	if result.Status != "rollback_failed" || err == nil || !strings.Contains(err.Error(), "E489") {
		t.Fatalf("rollback health = %#v %v", result, err)
	}
}

func TestPromoteRollbackFailed(t *testing.T) {
	var events []string
	steps := canonicalSteps(func(name string, _ *State) error {
		if name == "health" {
			return errors.New("unhealthy")
		}
		return nil
	}, func(name string, _ *State) error {
		events = append(events, name)
		if name == "apply" {
			return errors.New("restore tar")
		}
		return nil
	})
	result, err := (Engine{Steps: steps}).Run(context.Background(), &State{SnapshotTag: "pre-promote-2-bbbbbbbb", BackupID: "backup-2"})
	if result.Status != "rollback_failed" || err == nil || !strings.Contains(err.Error(), "E489") ||
		!strings.Contains(err.Error(), "pre-promote-2-bbbbbbbb") || !strings.Contains(err.Error(), "backup-2") {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
	if strings.Join(events, ",") != "health,migrate,apply" {
		t.Fatalf("undo continued after failure: %v", events)
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
