package imageswitch

// run.go — Plan and Run: the order of the steps and what happens when one fails.
//
// Purpose: one entry point per flag combination of `nself db image switch`.
// Inputs: Config, Deps, RunOptions. Outputs: a Report or a coded error.
// Constraints: a step that fails before env is written undoes the switch
// (undo): the stack runs again on the old volume and the new volume is gone. A
// step that fails after that leaves the state incomplete so --resume or
// --rollback decides. A resumed run repeats the step the state says was not
// finished; every step is safe to repeat.

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// planSteps names the steps, in order, for --plan.
var planSteps = []string{
	"preflight: postgres runs, same-major pgvector image, disk checked twice",
	"quiesce: every service but postgres stops, databases become read-only, other sessions end",
	"dump: globals and every database in one exported snapshot each, with a capture of rows, schema, sequences, roles and grants",
	"restore: into a NEW volume inside a one-shot server (no init scripts, no network)",
	"verify: the restored cluster equals the capture (E510 if not)",
	"recheck: the original has not changed since the dump (E510 if it has)",
	"switch: stop postgres, write POSTGRES_IMAGE and POSTGRES_DATA_VOLUME, regenerate compose and the lock, start",
}

// Plan reports what a run would do and changes nothing. The report is returned
// with the E509 error when a check failed, so the caller can show the checks.
func (s *Switch) Plan(ctx context.Context) (*Report, error) {
	st, err := LoadState(s.cfg.ProjectDir)
	if err != nil {
		return nil, err
	}
	if st != nil && !st.Phase.Complete() {
		return nil, incompleteError(st)
	}
	r, err := s.preflight(ctx)
	if err != nil {
		return nil, err
	}
	rep := &Report{Status: "planned", Preflight: r.Checks, Steps: planSteps, OldImage: r.Info.Image, OldVolume: r.Info.DataVolume}
	if r.Already {
		rep.Status, rep.Steps = "already-pgvector", nil
		return rep, nil
	}
	rep.NewImage, rep.DataBytes = r.Target, r.DataBytes
	rep.Stage = s.stage
	rep.DowntimeSeconds = estimateSeconds(r.DataBytes)
	if r.Cluster != nil {
		rep.Databases = len(r.Cluster.Databases)
		rep.Dumps = append(rep.Dumps, filepath.Join(s.stage, "globals.old.sql"))
		for i, d := range r.Cluster.Databases {
			rep.Dumps = append(rep.Dumps, filepath.Join(s.stage, dbFile(i, "dump"))+" ("+d.Name+")")
		}
	}
	if ferr := r.failed(); ferr != nil {
		return rep, ferr
	}
	name := s.newVolumeName(r)
	rep.NewVolume = name
	build, err := s.d.Applier.Plan(ctx, map[string]string{EnvImage: r.Target, EnvDataVolume: name})
	if err != nil {
		return rep, fmt.Errorf("planning the compose and lock change: %w", err)
	}
	rep.Build = build
	return rep, nil
}

// incompleteError is E512 for an unfinished switch.
func incompleteError(st *State) error {
	return coded("E512", "an image switch is already in progress",
		fmt.Sprintf("a switch to %s stopped in phase %q", st.NewImage, st.Phase), "")
}

// newVolumeName is <compose project>_postgres_data_pgvector_<utc timestamp>.
func (s *Switch) newVolumeName(r *preflightResult) string {
	project := r.Info.ComposeProject
	if project == "" {
		project = s.cfg.ProjectName
	}
	return project + "_postgres_data_pgvector_" + s.d.Now().UTC().Format("20060102t150405")
}

// newState builds the state of a fresh switch.
func (s *Switch) newState(r *preflightResult) (*State, error) {
	var network string
	for _, n := range r.Info.Networks {
		if n != "bridge" && n != "host" && n != "none" {
			network = n
			break
		}
	}
	if network == "" {
		return nil, coded("E509", "image switch preflight failed",
			"the postgres container is not on a user-defined docker network, so the dump cannot reach it", "")
	}
	project := r.Info.ComposeProject
	if project == "" {
		project = s.cfg.ProjectName
	}
	prev := make(map[string]string)
	for k, v := range s.cfg.PrevEnv {
		prev[k] = v
	}
	now := s.d.Now().UTC()
	return &State{
		RunID: "sw-" + now.Format("20060102t150405"), Container: s.cfg.Container, Network: network, ComposeProject: project,
		BootUser: r.Cluster.Bootstrap, Major: r.Cluster.Major, OldImage: r.Info.Image, NewImage: r.Target,
		OldVolume: r.Info.DataVolume, NewVolume: s.newVolumeName(r), Databases: r.Cluster.names(),
		PrevEnv: prev, DataBytes: r.DataBytes, StartedAt: now,
	}, nil
}

// Run performs the switch, or continues an incomplete one with Resume.
func (s *Switch) Run(ctx context.Context, o RunOptions) (*Report, error) {
	existing, err := LoadState(s.cfg.ProjectDir)
	if err != nil {
		return nil, err
	}
	switch {
	case existing != nil && !existing.Phase.Complete():
		if !o.Resume {
			return nil, withNote(incompleteError(existing), "run with --resume to finish it or --rollback to go back")
		}
		s.st, s.resumed = existing, true
		s.say("resuming from phase %q", existing.Phase)
	case o.Resume:
		return nil, fmt.Errorf("there is no incomplete image switch to resume")
	default:
		r, err := s.preflight(ctx)
		if err != nil {
			return nil, err
		}
		if r.Already {
			_ = s.cleanStage() // a leftover from a crash between done and cleanup holds a full dump
			return &Report{Status: "already-pgvector", OldImage: r.Info.Image, Preflight: r.Checks,
				Warnings: []string{"postgres is already on pgvector; nothing was changed"}}, nil
		}
		if ferr := r.failed(); ferr != nil {
			return nil, ferr
		}
		if s.st, err = s.newState(r); err != nil {
			return nil, err
		}
		if err := s.persist(PhaseStarted); err != nil {
			return nil, err
		}
	}
	return s.drive(ctx)
}

// step is one unit of the pre-switch sequence.
type step struct {
	from Phase
	run  func(context.Context) error
	to   Phase
}

// drive runs the steps the state has not finished.
func (s *Switch) drive(ctx context.Context) (*Report, error) {
	pre := []step{
		{PhaseStarted, s.quiesce, PhaseQuiesced},
		{PhaseQuiesced, s.dump, PhaseDumped},
		{PhaseDumped, s.restore, PhaseRestored},
		{PhaseRestored, s.verify, PhaseVerified},
		{PhaseVerified, s.recheck, PhaseRechecked},
	}
	for _, p := range pre {
		if s.st.Phase != p.from {
			continue
		}
		err := ctx.Err()
		if err == nil {
			err = p.run(ctx)
		}
		if err == nil {
			err = s.persist(p.to)
		}
		if err != nil {
			return nil, s.abort(ctx, err)
		}
	}
	for _, from := range []Phase{PhaseRechecked, PhaseSwitching} {
		if s.st.Phase != from {
			continue
		}
		// A resumed switch re-checks the original: it may have run for hours since the recheck.
		if s.resumed && from == PhaseRechecked {
			if err := s.recheck(ctx); err != nil {
				return nil, s.abort(ctx, err)
			}
		}
		if err := s.switchover(ctx); err != nil {
			return nil, err
		}
	}
	if s.st.Phase == PhaseSwitched {
		if err := s.start(ctx); err != nil {
			return nil, withNote(err, "The old volume is untouched. Run `nself db image switch --to pgvector --resume` to retry or `nself db image switch --rollback` to go back.")
		}
	}
	if s.st.Phase != PhaseDone {
		return nil, fmt.Errorf("the switch stopped in phase %q", s.st.Phase)
	}
	return s.report(), nil
}

// abort undoes a switch that failed before env was written.
func (s *Switch) abort(ctx context.Context, cause error) error {
	s.say("failed (%s); undoing", firstLine(cause.Error()))
	if uerr := s.undo(ctx); uerr != nil {
		return withNote(cause, "Cleanup failed ("+s.redact(uerr.Error(), 3)+"); run `nself db image switch --rollback`.")
	}
	return withNote(cause, "Nothing was switched and the old stack is running again.")
}

// firstLine is the first line of s.
func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

// report is the result of a finished switch.
func (s *Switch) report() *Report {
	st := s.st
	tolerated := append([]string(nil), st.Tolerated...)
	sort.Strings(tolerated)
	return &Report{
		Status: "switched", Phase: st.Phase, OldImage: st.OldImage, NewImage: st.NewImage, OldVolume: st.OldVolume, NewVolume: st.NewVolume,
		Databases: len(st.Databases), Tables: st.Tables, Sequences: st.Sequences, DataBytes: st.DataBytes,
		DowntimeSeconds: int(s.d.Now().Sub(st.StartedAt).Seconds()), Tolerated: tolerated,
		NextSteps: []string{
			"the old volume " + st.OldVolume + " was kept and its databases are read-only; to go back: nself db image switch --rollback --yes",
			"when you are satisfied, remove the old volume by hand: docker volume rm " + st.OldVolume,
		},
	}
}
