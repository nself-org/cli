package imageswitch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

func code(t *testing.T, err error) string {
	t.Helper()
	d := errs.Describe(err)
	if d == nil {
		t.Fatal("no error")
	}
	return d.Code
}

func TestRunHappyPath(t *testing.T) {
	f, s := newFake(t)
	rep, err := s.Run(context.Background(), RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "switched" || rep.NewImage != newImage || rep.OldVolume != "proj_postgres_data" {
		t.Fatalf("report = %+v", rep)
	}
	if !strings.HasPrefix(rep.NewVolume, "proj_postgres_data_pgvector_20261012t120000") {
		t.Errorf("new volume = %q", rep.NewVolume)
	}
	st, err := LoadState(f.dir)
	if err != nil || st.Phase != PhaseDone {
		t.Fatalf("state = %+v, %v", st, err)
	}
	if len(f.envApplied) != 1 || f.envApplied[0][EnvImage] != newImage || f.envApplied[0][EnvDataVolume] != rep.NewVolume {
		t.Errorf("applied env = %v", f.envApplied)
	}
	if got := f.labels[rep.NewVolume]["com.docker.compose.volume"]; got != "postgres_data" {
		t.Errorf("new volume is not labelled for compose: %v", f.labels[rep.NewVolume])
	}
	if !f.volumes["proj_postgres_data"] {
		t.Error("the old volume must be kept")
	}
	if len(f.roSet) != 2 {
		t.Errorf("both databases stay read-only on the old volume after the switch, got %v", f.roSet)
	}
	if _, err := os.Stat(filepath.Join(f.dir, ".nself", "image-switch", "stage")); !os.IsNotExist(err) {
		t.Error("the stage must be removed after success")
	}
	if rep.Tables != 2 || rep.Sequences != 2 || len(rep.Tolerated) != 1 {
		t.Errorf("counts: tables %d sequences %d tolerated %v", rep.Tables, rep.Sequences, rep.Tolerated)
	}
	// Order: quiesce before the dump, the dump before the restore, the switch last.
	order := []string{"stack.stop: auth,hasura,nginx", "dump.sh", "volume.create", "restore.sh", "recheck.sh", "stack.stop: postgres", "apply:", "stack.up: postgres,auth,hasura,nginx\n"}
	at := -1
	for _, want := range order {
		found := -1
		for i := at + 1; i < len(f.calls); i++ {
			if strings.Contains(f.calls[i]+"\n", want) {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("%q missing or out of order in %v", want, f.calls)
		}
		at = found
	}
}

func TestRunRefusesWhileIncompleteAndAlreadyPgvector(t *testing.T) {
	f, s := newFake(t)
	f.pgvector = true
	rep, err := s.Run(context.Background(), RunOptions{})
	if err != nil || rep.Status != "already-pgvector" {
		t.Fatalf("already: %+v %v", rep, err)
	}
	if len(f.envApplied) != 0 || f.called("stack.stop") {
		t.Error("an already-pgvector run must change nothing")
	}
	f2, s2 := newFake(t)
	st := &State{Phase: PhaseDumped, NewImage: newImage}
	if err := st.save(f2.dir, s2.d.Now()); err != nil {
		t.Fatal(err)
	}
	_, err = s2.Run(context.Background(), RunOptions{})
	if code(t, err) != "E512" || !strings.Contains(err.Error(), "--resume") {
		t.Fatalf("incomplete run err = %v", err)
	}
}

func TestRunAbortsAndUndoes(t *testing.T) {
	for _, tc := range []struct {
		name, mutate, fail, want string
	}{
		{"verify mismatch", "restore", "", "E510"},
		{"original changed after the dump", "recheck", "", "E510"},
		{"restore failure", "", "restore.sh", "E511"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s := newFake(t)
			f.mutate, f.failStage, f.failErr = tc.mutate, tc.fail, "could not access file $libdir/plperl"
			_, err := s.Run(context.Background(), RunOptions{})
			if got := code(t, err); got != tc.want {
				t.Fatalf("code = %s (%v), want %s", got, err, tc.want)
			}
			if !strings.Contains(err.Error(), "Nothing was switched") {
				t.Errorf("error should say nothing was switched: %v", err)
			}
			if len(f.envApplied) != 0 {
				t.Error("env was written")
			}
			if len(f.roSet) != 0 {
				t.Errorf("read-only not reset: %v", f.roSet)
			}
			for v := range f.volumes {
				if strings.Contains(v, "pgvector") {
					t.Errorf("new volume %s was not removed", v)
				}
			}
			if !f.running["hasura"] || !f.running["auth"] || !f.running["nginx"] || !f.running["postgres"] {
				t.Errorf("stack not running again: %v", f.running)
			}
			if _, err := LoadState(f.dir); err != nil {
				t.Fatal(err)
			}
			if st, _ := LoadState(f.dir); st != nil {
				t.Errorf("state must be removed after a clean undo, got %+v", st)
			}
			for _, secret := range []string{fakeHash, fakePassword} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("error leaks %q: %v", secret, err)
				}
			}
		})
	}
}

func TestRunFailureLeavesNoStateWhenUndoFails(t *testing.T) {
	f, s := newFake(t)
	f.mutate = "restore"
	f.sessions = "0"
	// A stack that cannot restart makes the undo fail: the state must stay, so the guard keeps blocking.
	s.d.Stack = failingUp{f}
	_, err := s.Run(context.Background(), RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "--rollback") {
		t.Fatalf("err = %v", err)
	}
	if st, _ := Incomplete(f.dir); st == nil {
		t.Error("an undo that failed must leave the switch incomplete")
	}
}

type failingUp struct{ *fakeEnv }

func (failingUp) Up(context.Context, ...string) error { return errors.New("compose up failed") }

func TestQuiesceFailsWhenSessionsRemain(t *testing.T) {
	f, s := newFake(t)
	f.sessions = "2"
	_, err := s.Run(context.Background(), RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "still connected") {
		t.Fatalf("err = %v", err)
	}
	if len(f.roSet) != 0 || len(f.envApplied) != 0 {
		t.Error("undo must reset read-only and write nothing")
	}
}

func TestResumeAfterKill(t *testing.T) {
	for _, point := range []string{"restore", "apply", "start"} {
		t.Run(point, func(t *testing.T) {
			f, s := newFake(t)
			f.killAt = point
			if k := runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) }); k == nil || k.at != point {
				t.Fatalf("expected a kill at %s, got %v", point, k)
			}
			st, incomplete := Incomplete(f.dir)
			if !incomplete {
				t.Fatalf("a killed switch must be incomplete (state %+v)", st)
			}
			// Another run is refused; resume finishes from the state.
			if _, err := s.Run(context.Background(), RunOptions{}); code(t, err) != "E512" {
				t.Fatalf("plain run after a kill: %v", err)
			}
			rep, err := New(s.cfg, s.d).Run(context.Background(), RunOptions{Resume: true})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Status != "switched" {
				t.Fatalf("report = %+v", rep)
			}
			if _, incomplete := Incomplete(f.dir); incomplete {
				t.Error("still incomplete after resume")
			}
			if point == "restore" && !f.called("volume.rm: "+rep.NewVolume) {
				t.Error("a resumed restore must start from an empty volume")
			}
		})
	}
}

func TestResumeWithoutStateIsRefused(t *testing.T) {
	_, s := newFake(t)
	if _, err := s.Run(context.Background(), RunOptions{Resume: true}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRollback(t *testing.T) {
	f, s := newFake(t)
	if _, err := s.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rollback(context.Background(), false); code(t, err) != "E403" {
		t.Fatalf("rollback after success without --yes: %v", err)
	}
	f.calls = nil
	rep, err := s.Rollback(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "rolled-back" || !strings.Contains(strings.Join(rep.NextSteps, " "), rep.NewVolume) {
		t.Fatalf("report = %+v", rep)
	}
	last := f.envApplied[len(f.envApplied)-1]
	if last[EnvImage] != "" || last[EnvDataVolume] != "" {
		t.Errorf("rollback must restore the previous env, wrote %v", last)
	}
	if !f.volumes[rep.NewVolume] || !f.volumes["proj_postgres_data"] {
		t.Error("rollback must keep both volumes")
	}
	if len(f.roSet) != 0 {
		t.Errorf("rollback must reset read-only, left %v", f.roSet)
	}
	// postgres starts alone before the read-only reset, the whole stack after it.
	iPg, iReset, iAll := -1, -1, -1
	for i, c := range f.calls {
		switch {
		case c == "stack.up: postgres":
			iPg = i
		case strings.Contains(c, "RESET default_transaction_read_only") && iReset < 0:
			iReset = i
		case c == "stack.up: postgres,auth,hasura,nginx":
			iAll = i
		}
	}
	if !(iPg >= 0 && iPg < iReset && iReset < iAll) {
		t.Errorf("rollback order wrong: postgres %d reset %d all %d in %v", iPg, iReset, iAll, f.calls)
	}
	if st, _ := LoadState(f.dir); st == nil || st.Phase != PhaseRolledBack {
		t.Errorf("state = %+v", st)
	}
	if _, incomplete := Incomplete(f.dir); incomplete {
		t.Error("a rolled-back switch is complete")
	}
	// A new switch may start after a rollback.
	f.pgvector = false
	s.d.Now = func() time.Time { return time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC) }
	if _, err := s.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatalf("switch after rollback: %v", err)
	}
}

func TestRollbackBeforeSwitchUndoes(t *testing.T) {
	f, s := newFake(t)
	f.killAt = "restore"
	runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) })
	rep, err := New(s.cfg, s.d).Rollback(context.Background(), false)
	if err != nil || rep.Status != "rolled-back" {
		t.Fatalf("%+v %v", rep, err)
	}
	if len(f.envApplied) != 0 || len(f.roSet) != 0 || !f.running["hasura"] {
		t.Errorf("env %v ro %v running %v", f.envApplied, f.roSet, f.running)
	}
	if st, _ := LoadState(f.dir); st != nil {
		t.Errorf("state must be gone, got %+v", st)
	}
	if _, err := s.Rollback(context.Background(), true); err == nil {
		t.Error("rollback with no switch must say so")
	}
}

func TestPlanChangesNothing(t *testing.T) {
	f, s := newFake(t)
	rep, err := s.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "planned" || rep.NewVolume == "" || rep.Build == nil || rep.DowntimeSeconds < 60 || len(rep.Steps) == 0 {
		t.Fatalf("plan = %+v", rep)
	}
	if f.called("stack.stop") || f.called("ALTER DATABASE") || len(f.envApplied) != 0 {
		t.Errorf("plan changed something: %v", f.calls)
	}
	if _, err := os.Stat(StatePath(f.dir)); !os.IsNotExist(err) {
		t.Error("plan wrote a state")
	}
}
