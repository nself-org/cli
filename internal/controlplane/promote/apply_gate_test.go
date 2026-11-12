package promote

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
)

func appliedState(r deploy.Remote) *State {
	return &State{Values: map[string]any{
		"apply_remotes": map[string]deploy.Remote{r.Label(): r},
		"apply_paths":   map[string]string{r.Label(): "/srv/app"},
	}}
}

// TestWire15GatedHealthRunsGateBeforeDoctor: the deferred apply's gate (hasura
// metadata apply) runs on every applied host before the doctor health check.
func TestWire15GatedHealthRunsGateBeforeDoctor(t *testing.T) {
	r := &wire15SchemaRemote{migrated: true}
	base := &controlplane.RemoteApply{StrictHasura: true}
	step := GatedHealthStep(r, "/srv/app", func() *controlplane.RemoteApply { return base })
	if step.Name != "health" {
		t.Fatalf("name = %q, want health", step.Name)
	}
	if err := step.Run(context.Background(), appliedState(r)); err != nil {
		t.Fatalf("gated health: %v", err)
	}
	joined := strings.Join(r.calls, "\n")
	metadata, doctor := strings.Index(joined, "hasura metadata apply"), strings.LastIndex(joined, "nself doctor")
	if metadata < 0 || doctor <= metadata {
		t.Fatalf("order is not gate, then doctor:\n%s", joined)
	}
}

// TestWire15GateAppliedStopsBeforeDoctor: a failing strict gate stops the
// health step before the doctor runs; no applied hosts gate nothing.
func TestWire15GateAppliedStopsBeforeDoctor(t *testing.T) {
	r := &wire15SchemaRemote{oldFiles: true, migrated: true}
	step := GatedHealthStep(r, "/srv/app", func() *controlplane.RemoteApply { return &controlplane.RemoteApply{StrictHasura: true} })
	if err := step.Run(context.Background(), appliedState(r)); err == nil {
		t.Fatal("gate passed on an inconsistent host")
	}
	if strings.Contains(strings.Join(r.calls, "\n"), "nself doctor") {
		t.Fatalf("doctor ran after a failed gate:\n%s", strings.Join(r.calls, "\n"))
	}
	if err := GateApplied(context.Background(), &State{}, nil); err != nil {
		t.Fatalf("no applied hosts: %v", err)
	}
}

// emptyHost names no compose set under its remote path.
type emptyHost struct{ wire15SchemaRemote }

func (*emptyHost) Run(context.Context, string) ([]byte, error) { return nil, nil }

func TestWire15GateAppliedNeedsComposeSet(t *testing.T) {
	r := &emptyHost{}
	err := GateApplied(context.Background(), appliedState(r), nil)
	if err == nil || !strings.Contains(err.Error(), "names no compose set") {
		t.Fatalf("err = %v, want a missing compose set", err)
	}
}

// TestWire15DeferredApplyStepUndo: the apply step undoes files first and
// restarts only hosts whose files were restored; nothing recorded is a no-op.
func TestWire15DeferredApplyStepUndo(t *testing.T) {
	ran := false
	step := DeferredApplyStep(func(context.Context, *State) error { ran = true; return nil })
	if step.Name != "apply" || step.Undo == nil || step.Restart == nil {
		t.Fatalf("step = %+v", step)
	}
	st := &State{}
	if err := step.Run(context.Background(), st); err != nil || !ran {
		t.Fatalf("run: %v ran=%v", err, ran)
	}
	if err := step.Undo(context.Background(), st); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if err := step.Restart(context.Background(), st); err != nil {
		t.Fatalf("restart: %v", err)
	}
	st.PreDeployTar = map[string]string{"primary": "/srv/app/.nself/backups/pre.tar.gz"}
	if err := step.Undo(context.Background(), st); err == nil || !strings.Contains(err.Error(), "no remote") {
		t.Fatalf("undo without a remote: %v", err)
	}
}
