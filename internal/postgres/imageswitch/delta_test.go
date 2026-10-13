package imageswitch

// Tests for a switchover that fails after the phase is `switching`, and for the
// checks that keep a resume from working on the wrong cluster.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// killedAtApply runs a switch that dies in the env rewrite and leaves phase switching
// with the env already pointing at the new volume (the worst case for a resume).
func killedAtApply(t *testing.T) (*fakeEnv, *Switch, *State) {
	t.Helper()
	f, s := newFake(t)
	f.killAt = "apply"
	runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) })
	st, err := LoadState(f.dir)
	if err != nil || st == nil || st.Phase != PhaseSwitching {
		t.Fatalf("state = %+v, %v; want phase switching", st, err)
	}
	f.curVolume, f.pgvector = st.NewVolume, true // the earlier attempt wrote the env
	f.envApplied = nil
	return f, s, st
}

func TestResumeFromSwitchingFailureRestoresThePreviousEnv(t *testing.T) {
	f, s, st := killedAtApply(t)
	f.stopPGErr = errors.New("docker: stop timed out")
	_, err := New(s.cfg, s.d).Run(context.Background(), RunOptions{Resume: true})
	if err == nil || !strings.Contains(err.Error(), "put back") {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.envApplied); n != 1 || f.envApplied[0][EnvImage] != "" || f.envApplied[0][EnvDataVolume] != "" {
		t.Errorf("the previous env must be written back: %v", f.envApplied)
	}
	if f.dataVolume() != st.OldVolume || f.pgvector || !f.pgRunning || !f.running["hasura"] {
		t.Errorf("the old stack must run on the old volume: volume %s pgvector %v running %v", f.dataVolume(), f.pgvector, f.running)
	}
	if f.volumes[st.NewVolume] {
		t.Error("the unused new volume must be removed")
	}
	if got, _ := LoadState(f.dir); got == nil || got.Phase != PhaseRolledBack {
		t.Errorf("state = %+v, want rolled-back", got)
	}
}

func TestFailedRollbackInsideSwitchoverKeepsTheNewVolumeAndStaysIncomplete(t *testing.T) {
	f, s := newFake(t)
	f.applyErr = errors.New("rendering compose: boom")
	f.healthErr = errors.New("postgres is unhealthy")
	_, err := s.Run(context.Background(), RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "also failed") {
		t.Fatalf("err = %v", err)
	}
	st, inc := Incomplete(f.dir)
	if !inc || st == nil {
		t.Fatal("a rollback that could not finish must leave the switch incomplete")
	}
	if !f.volumes[st.NewVolume] {
		t.Error("the new volume must be kept while the old stack is not back")
	}
}

func TestCancelledContextStillRestoresThePreviousEnv(t *testing.T) {
	f, s := newFake(t)
	f.applyErr = errors.New("rendering compose: boom")
	ctx, cancel := context.WithCancel(context.Background())
	f.onDump = func() {} // keep the dump quiet; cancel once the switchover is reached
	s.d.Stack = cancelOnStop{f, cancel}
	_, err := s.Run(ctx, RunOptions{})
	if err == nil {
		t.Fatal("want an error")
	}
	if !f.pgRunning || f.pgvector || !f.running["hasura"] {
		t.Errorf("an interrupt must not leave the stack down: running %v", f.running)
	}
}

// cancelOnStop cancels the context when postgres alone is stopped (the switchover).
type cancelOnStop struct {
	*fakeEnv
	cancel context.CancelFunc
}

func (c cancelOnStop) Stop(ctx context.Context, services ...string) error {
	if len(services) == 1 && services[0] == "postgres" {
		c.cancel()
	}
	return c.fakeEnv.Stop(ctx, services...)
}

func TestQuiesceRefusesAClusterOnAnotherVolume(t *testing.T) {
	f, s := newFake(t)
	f.killAt = "stop"
	runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) })
	st, _ := LoadState(f.dir)
	st.Phase = PhaseStarted // what a failed undo leaves
	if err := st.save(f.dir, s.d.Now()); err != nil {
		t.Fatal(err)
	}
	f.curVolume = "proj_postgres_data_pgvector_x" // the env still names the new volume
	f.calls = nil
	_, err := New(s.cfg, s.d).Run(context.Background(), RunOptions{Resume: true})
	if err == nil || !strings.Contains(err.Error(), "not running on the old volume") {
		t.Fatalf("err = %v", err)
	}
	if f.called("stack.stop") || f.called("SET default_transaction_read_only = on") {
		t.Errorf("nothing may be stopped or locked on the wrong cluster: %v", f.calls)
	}
}

// wrongVolumeRuntime reports postgres as stopped on another volume once the restore started.
type wrongVolumeRuntime struct{ *fakeEnv }

func (r wrongVolumeRuntime) Inspect(ctx context.Context, name string) (ContainerInfo, error) {
	ci, err := r.fakeEnv.Inspect(ctx, name)
	if r.fakeEnv.called("restore.sh") {
		ci.Running, ci.DataVolume = false, "proj_postgres_data_pgvector_x"
	}
	return ci, err
}

func TestUndoDoesNotStartServicesOnTheWrongVolume(t *testing.T) {
	f, s := newFake(t)
	f.mutate = "restore" // forces the undo
	s.d.Runtime = wrongVolumeRuntime{f}
	_, err := s.Run(context.Background(), RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "--rollback") {
		t.Fatalf("err = %v", err)
	}
	if f.called("stack.up: auth,hasura,nginx") {
		t.Errorf("services must not start on a cluster that is not the old volume: %v", f.calls)
	}
	if st, inc := Incomplete(f.dir); !inc || st.Phase != PhaseStarted {
		t.Errorf("state = %+v, want incomplete in phase started", st)
	}
}
