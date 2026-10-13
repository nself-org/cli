package imageswitch

// Tests for review round 1 (CR and QA of PR 754): databases that appear mid-run,
// failures in the last step, ordering when postgres is not ready, row data in
// error text, resume after kills at every step, cancellation.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestQuiesceListsTheDatabasesAgain(t *testing.T) {
	f, s := newFake(t)
	f.newDBOnStop = true
	rep, err := s.Run(context.Background(), RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Databases != 3 || !f.roSet["late"] {
		t.Errorf("a database created after the preflight must be dumped and made read-only: databases %d ro %v", rep.Databases, f.roSet)
	}
}

func TestDumpDetectsADatabaseCreatedDuringTheDump(t *testing.T) {
	f, s := newFake(t)
	f.newDBAtDump = true
	_, err := s.Run(context.Background(), RunOptions{})
	if code(t, err) != "E510" || !strings.Contains(err.Error(), "late") {
		t.Fatalf("err = %v", err)
	}
	if len(f.envApplied) != 0 || len(f.roSet) != 0 {
		t.Errorf("must be undone: env %v ro %v", f.envApplied, f.roSet)
	}
}

func TestStopFailureBeforeTheSwitchUndoes(t *testing.T) {
	f, s := newFake(t)
	f.stopPGErr = errors.New("docker compose stop: exit status 1")
	_, err := s.Run(context.Background(), RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "Nothing was switched") {
		t.Fatalf("err = %v", err)
	}
	if len(f.envApplied) != 0 || len(f.roSet) != 0 || !f.running["hasura"] || !f.running["postgres"] {
		t.Errorf("stack must run again: env %v ro %v running %v", f.envApplied, f.roSet, f.running)
	}
	if st, _ := LoadState(f.dir); st != nil {
		t.Errorf("state must be gone: %+v", st)
	}
}

func TestApplyFailureRollsBackToTheOldStack(t *testing.T) {
	f, s := newFake(t)
	f.applyErr = errors.New("rendering compose: boom")
	_, err := s.Run(context.Background(), RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "put back") {
		t.Fatalf("err = %v", err)
	}
	if len(f.envApplied) != 1 || f.envApplied[0][EnvImage] != "" || f.envApplied[0][EnvDataVolume] != "" {
		t.Errorf("the previous env must be written back: %v", f.envApplied)
	}
	if !f.running["postgres"] || !f.running["hasura"] || len(f.roSet) != 0 || f.pgvector {
		t.Errorf("the old stack must run, writable: running %v ro %v pgvector %v", f.running, f.roSet, f.pgvector)
	}
	st, _ := LoadState(f.dir)
	if st == nil || st.Phase != PhaseRolledBack {
		t.Errorf("state = %+v, want rolled-back (the new volume is kept)", st)
	}
	if _, inc := Incomplete(f.dir); inc {
		t.Error("a rolled-back switch is complete: other commands are not blocked")
	}
}

func TestOtherServicesDoNotStartOnAnUnhealthyPostgres(t *testing.T) {
	f, s := newFake(t)
	f.mutate = "restore" // forces the undo
	f.healthErr = errors.New("unhealthy")
	f.pgRunning = true
	// Postgres is reported stopped at undo time, so undo must start it and check it.
	runtimeStopped := stoppedRuntime{f}
	s.d.Runtime = runtimeStopped
	_, err := s.Run(context.Background(), RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "--rollback") {
		t.Fatalf("err = %v", err)
	}
	for _, c := range f.calls {
		if c == "stack.up: auth,hasura,nginx" {
			t.Fatalf("the writers must not start against an unhealthy postgres: %v", f.calls)
		}
	}
	if st, inc := Incomplete(f.dir); !inc || st == nil {
		t.Error("an undo that could not finish must leave the switch incomplete")
	}
}

// stoppedRuntime reports the postgres container as not running once the restore has started.
type stoppedRuntime struct{ *fakeEnv }

func (r stoppedRuntime) Inspect(ctx context.Context, name string) (ContainerInfo, error) {
	ci, err := r.fakeEnv.Inspect(ctx, name)
	if r.fakeEnv.called("restore.sh") {
		ci.Running = false
	}
	return ci, err
}

func TestRollbackFromSwitchedNeedsYes(t *testing.T) {
	f, s := newFake(t)
	f.killAt = "start"
	runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) })
	if st, _ := LoadState(f.dir); st == nil || st.Phase != PhaseSwitched {
		t.Fatalf("state = %+v", st)
	}
	if _, err := New(s.cfg, s.d).Rollback(context.Background(), false); code(t, err) != "E403" {
		t.Fatalf("rollback of a switched project without --yes: %v", err)
	}
}

func TestResumeFromRecheckedChecksTheOriginalAgain(t *testing.T) {
	f, s := newFake(t)
	f.killAt = "apply"
	runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) })
	st, _ := LoadState(f.dir)
	st.Phase = PhaseRechecked
	if err := st.save(f.dir, s.d.Now()); err != nil {
		t.Fatal(err)
	}
	before := strings.Count(strings.Join(f.calls, "\n"), "recheck.sh")
	f.mutate = "recheck" // the original changed while the switch sat there
	_, err := New(s.cfg, s.d).Run(context.Background(), RunOptions{Resume: true})
	if code(t, err) != "E510" {
		t.Fatalf("a resume must notice the original changed: %v", err)
	}
	if after := strings.Count(strings.Join(f.calls, "\n"), "recheck.sh"); after != before+1 {
		t.Errorf("recheck ran %d times, want %d", after, before+1)
	}
}

func TestKillAtEveryStepResumes(t *testing.T) {
	for _, point := range []string{"stop", "dump", "recheck"} {
		t.Run(point, func(t *testing.T) {
			f, s := newFake(t)
			f.killAt = point
			if k := runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) }); k == nil {
				t.Fatalf("no kill at %s", point)
			}
			if _, inc := Incomplete(f.dir); !inc {
				t.Fatal("a killed switch is incomplete")
			}
			rep, err := New(s.cfg, s.d).Run(context.Background(), RunOptions{Resume: true})
			if err != nil || rep.Status != "switched" {
				t.Fatalf("resume: %+v %v", rep, err)
			}
			if len(f.envApplied) != 1 {
				t.Errorf("env applied %d times", len(f.envApplied))
			}
		})
	}
}

func TestResumeRemovesLeftoverOneShotsBeforeTheVolume(t *testing.T) {
	f, s := newFake(t)
	f.killAt = "restore"
	runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) })
	f.removed = nil
	if _, err := New(s.cfg, s.d).Run(context.Background(), RunOptions{Resume: true}); err != nil {
		t.Fatal(err)
	}
	var restore bool
	for _, n := range f.removed {
		restore = restore || n == "nself-imgswitch-proj-restore"
	}
	if !restore {
		t.Errorf("the restore container of the killed run must be removed by name: %v", f.removed)
	}
}

func TestCancelledContextUndoes(t *testing.T) {
	f, s := newFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	f.onDump = cancel // Ctrl-C arrives while the dump runs
	_, err := s.Run(ctx, RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "Nothing was switched") {
		t.Fatalf("err = %v", err)
	}
	if len(f.envApplied) != 0 || len(f.roSet) != 0 || !f.running["hasura"] {
		t.Errorf("an interrupt must undo: env %v ro %v running %v", f.envApplied, f.roSet, f.running)
	}
	if f.called("restore.sh") {
		t.Error("no step may start after the interrupt")
	}
}

func TestRedactDropsRowDataAndThePassword(t *testing.T) {
	_, s := newFake(t)
	in := "pg_restore: error: could not execute query: ERROR:  duplicate key value violates unique constraint \"u\"\nDETAIL:  Key (email)=(someone@example.invalid) already exists.\nCONTEXT:  COPY users, line 3: \"3\tsomeone@example.invalid\"\nconnecting with " + fakePassword
	out := s.redact(in, 6)
	for _, bad := range []string{"someone@example.invalid", fakePassword} {
		if strings.Contains(out, bad) {
			t.Errorf("%q survived: %q", bad, out)
		}
	}
	if !strings.Contains(out, "duplicate key value violates") {
		t.Errorf("the error itself must stay: %q", out)
	}
}

func TestClusterProblemsNameWhatCannotMove(t *testing.T) {
	c := sampleCluster(t)
	c.Templates = []string{"tpl"}
	c.Databases[0].Name = "a=b"
	got := strings.Join(clusterProblems(c), "; ")
	for _, want := range []string{"template databases tpl", "a=b: the name would be read as a connection string"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	c.Templates, c.Databases[0].Name = nil, "-x"
	if len(clusterProblems(c)) == 0 {
		t.Error("a name starting with - must be refused")
	}
}

func TestSettingsFromAlterSystemAreReportedNotRefused(t *testing.T) {
	f, s := newFake(t)
	s.d.Runtime = autoconfRuntime{f}
	rep, err := s.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range rep.Preflight {
		if c.Name == "settings" {
			found = c.OK && strings.Contains(c.Detail, "max_wal_size")
		}
	}
	if !found {
		t.Errorf("preflight = %+v", rep.Preflight)
	}
}

// autoconfRuntime answers the size query as if ALTER SYSTEM had set max_wal_size.
type autoconfRuntime struct{ *fakeEnv }

func (r autoconfRuntime) ExecSQL(ctx context.Context, c, u, db, sql string) (string, error) {
	if strings.Contains(sql, "pg_database_size") {
		return "0|1048576|max_wal_size|0\n", nil
	}
	return r.fakeEnv.ExecSQL(ctx, c, u, db, sql)
}

// upFailsOnce makes the first start of the stopped writers fail, as a compose error would.
type upFailsOnce struct {
	*fakeEnv
	failed bool
}

func (u *upFailsOnce) Up(ctx context.Context, services ...string) error {
	if !u.failed && len(services) > 0 && services[0] == "auth" {
		u.failed = true
		return errors.New("compose up failed")
	}
	return u.fakeEnv.Up(ctx, services...)
}

// TestResumeNeverSwitchesToAVolumeTheUndoRemoved is the Opus M1 repro: a failed undo that already
// removed the new volume must not leave a phase that lets --resume switch onto it.
func TestResumeNeverSwitchesToAVolumeTheUndoRemoved(t *testing.T) {
	f, s := newFake(t)
	up := &upFailsOnce{fakeEnv: f}
	s.d.Stack = up
	f.mutate = "recheck"
	if _, err := s.Run(context.Background(), RunOptions{}); err == nil || !strings.Contains(err.Error(), "--rollback") {
		t.Fatalf("expected the failed undo to say so: %v", err)
	}
	st, inc := Incomplete(f.dir)
	if !inc || st.Phase != PhaseStarted || st.NewVolumeCreated {
		t.Fatalf("a failed undo must restart from scratch: %+v", st)
	}
	f.mutate = ""
	restores := strings.Count(strings.Join(f.calls, "\n"), "restore.sh")
	rep, err := New(s.cfg, s.d).Run(context.Background(), RunOptions{Resume: true})
	if err != nil {
		t.Fatal(err)
	}
	if !f.volumes[rep.NewVolume] || f.labels[rep.NewVolume][labelRun] == "" {
		t.Errorf("the switch must end on a volume that exists and is labelled: %v", f.volumes)
	}
	if got := strings.Count(strings.Join(f.calls, "\n"), "restore.sh"); got != restores+1 {
		t.Errorf("the restore must run again on resume (%d -> %d)", restores, got)
	}
}

func TestSwitchoverRefusesAMissingOrForeignNewVolume(t *testing.T) {
	for _, tc := range []struct {
		name string
		mod  func(f *fakeEnv, st *State)
	}{
		{"gone", func(f *fakeEnv, st *State) { delete(f.volumes, st.NewVolume) }},
		{"foreign label", func(f *fakeEnv, st *State) { f.labels[st.NewVolume] = map[string]string{labelRun: "sw-other"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s := newFake(t)
			f.killAt = "apply"
			runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) })
			st, _ := LoadState(f.dir)
			tc.mod(f, st)
			st.Phase = PhaseRechecked
			if err := st.save(f.dir, s.d.Now()); err != nil {
				t.Fatal(err)
			}
			f.envApplied = nil
			if _, err := New(s.cfg, s.d).Run(context.Background(), RunOptions{Resume: true}); err == nil {
				t.Fatal("the switch must not go ahead")
			}
			if len(f.envApplied) != 0 {
				t.Errorf("env was written: %v", f.envApplied)
			}
		})
	}
}

func TestForeignVolumeIsNeverRemoved(t *testing.T) {
	f, s := newFake(t)
	f.killAt = "restore"
	runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) })
	st, _ := LoadState(f.dir)
	f.labels[st.NewVolume] = map[string]string{labelRun: "sw-copied-project"}
	if _, err := New(s.cfg, s.d).Rollback(context.Background(), false); err == nil || !f.volumes[st.NewVolume] {
		t.Fatalf("a volume without this run's label must survive: err %v volumes %v", err, f.volumes)
	}
}

func TestRollbackChecksTheDataVolumeItStarts(t *testing.T) {
	f, s := newFake(t)
	if _, err := s.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatal(err)
	}
	f.ignoreVolume = true // compose keeps mounting the new volume
	f.calls = nil
	_, err := s.Rollback(context.Background(), true)
	if code(t, err) != "E512" || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("err = %v", err)
	}
	if f.called("stack.up: postgres,auth") {
		t.Errorf("the other services must not start on the wrong volume: %v", f.calls)
	}
}

func TestSessionsAreEndedAgainBeforeTheRecheck(t *testing.T) {
	f, s := newFake(t)
	if _, err := s.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range f.calls {
		if strings.Contains(c, "pg_terminate_backend") {
			n++
		}
	}
	if n < 2 {
		t.Errorf("sessions are ended at the quiesce and again before the recheck, saw %d", n)
	}
}

// replRuntime answers the size query as if a subscription existed.
type replRuntime struct{ *fakeEnv }

func (r replRuntime) ExecSQL(ctx context.Context, c, u, db, sql string) (string, error) {
	if strings.Contains(sql, "pg_database_size") {
		return "0|1048576||1\n", nil
	}
	return r.fakeEnv.ExecSQL(ctx, c, u, db, sql)
}

func TestPreflightRefusesSubscriptionsAndSlots(t *testing.T) {
	f, s := newFake(t)
	s.d.Runtime = replRuntime{f}
	_, err := s.Run(context.Background(), RunOptions{})
	if code(t, err) != "E509" || !strings.Contains(err.Error(), "replication slots") {
		t.Fatalf("err = %v", err)
	}
}

func TestAlreadyPgvectorRemovesAStageLeftByACrash(t *testing.T) {
	f, s := newFake(t)
	f.pgvector = true
	if err := s.prepareStage(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.stage); !os.IsNotExist(err) {
		t.Error("a leftover stage holds a full dump and must be removed")
	}
}
