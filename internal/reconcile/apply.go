package reconcile

// apply.go — reconcile.Apply: plan, confirm, then write (EPIC P7-LIVE D6, D9).
//
// Purpose: apply exactly the plan that was shown. Apply recomputes the plan
// with the same function `--plan` uses, refuses a plan id that no longer
// matches (E450), asks for the confirmation D4 requires, and only then runs the
// write-mode build.
// Inputs: a Request and the operator's ApplyOptions. Outputs: the applied
// *Plan (and, from ApplyBuild, the build result the command summarises).
// Constraints: nothing is written before the plan id and the confirmation have
// passed. The secrets the build generates come from one seed shared by the
// plan, the re-check and the write, so the three renders agree byte for byte.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// Apply plans and applies req. See ApplyBuild.
func Apply(ctx context.Context, req Request, opt ApplyOptions) (*Plan, error) {
	p, _, err := ApplyBuild(ctx, req, opt)
	return p, err
}

// buildBuild is a test seam for nbuild.Build.
var buildBuild = nbuild.Build

// ApplyBuild is Apply that also returns the write-mode build result.
//
// One locked render (EPIC ruling B): the project lock is taken first and held
// to the end; the project is rendered once in plan mode and that render is the
// plan; the plan id is checked; the operator confirms; and the write build is
// then held to exactly that render (nbuild.BuildOptions.Expect), so a value
// edited while the prompt waited cannot reach the disk. A re-render right before
// the write refuses (E450) a change made during the prompt before anything is
// written. The compat switch is read once, first: a project file cannot toggle
// it mid-command.
func ApplyBuild(ctx context.Context, req Request, opt ApplyOptions) (*Plan, *nbuild.BuildResult, error) {
	// compat.V15(P7-LIVE-03): a prod-class or hand-edited change proceeds with a notice -> refused with E403 without --yes or --force
	v15 := compat.V15()
	defer nbuild.SnapshotEnv()() // the write build exports the project's env; leave the process as found
	if len(req.Seed) == 0 {
		seed := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, seed); err != nil {
			return nil, nil, fmt.Errorf("drawing the secret seed: %w", err)
		}
		req.Seed = seed
	}
	release, err := nbuild.AcquireBuildLock(ctx, req.ProjectDir)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	// P7-LIVE-04 (EPIC D5): the generated-state record tells a generated change
	// from a human one. A missing record is the first run on an existing
	// project: nothing is detected (never block), the apply records what it
	// wrote and warns once below. A record a caller already supplied (tests,
	// later callers) is used as given.
	state, err := LoadGeneratedState(req.ProjectDir)
	if err != nil {
		return nil, nil, err
	}
	if req.HandEdited == nil {
		req.HandEdited = state.HandEditedFn(req.ProjectDir)
	}
	c, err := compute(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	p := c.plan
	// P7-LIVE-04: the diff of every hand-edited file comes first, so the
	// operator sees exactly what a --force would discard (v1.5) or discard
	// silently (v1.4's warning below) before the plan summary and the gate.
	if err := warnHandEdits(req, p, c, v15); err != nil {
		return nil, nil, err
	}
	if opt.PlanID != "" {
		if c.generates {
			return nil, nil, errs.New("E451", "this build generates secrets, so --plan-id cannot bind it").
				WithWhy("random values cannot be reproduced from a plan id, so a plan id would not describe what is written").
				WithFix("set the secrets in .env.secrets first, or run nself build and confirm at the prompt (or with --yes, without --plan-id)")
		}
		if hasEffect(p, EffectPluginInstall) || hasEffect(p, EffectPluginRemove) {
			return nil, nil, errs.New("E453", "this plan installs or removes plugins, so --plan-id cannot bind it").
				WithWhy("what a plugin brings is only known after it runs, so the id cannot describe the render that is written").
				WithFix("run nself build --yes without --plan-id (the render after the plugin changes is printed and held), or install or remove the plugins first")
		}
		if opt.PlanID != p.PlanID {
			return nil, nil, errs.New("E450", "the plan id does not match what this command would change now").
				WithWhy(fmt.Sprintf("plan %s was passed, the project now plans %s: an input changed since the plan was shown", short(opt.PlanID), short(p.PlanID))).
				WithFix("re-run nself build --plan and pass the new plan_id")
		}
	}
	if req.Stderr != nil && !p.Empty && (p.RequiresConfirmation || len(HandEditedPaths(*p)) > 0) {
		if err := RenderHuman(req.Stderr, *p); err != nil {
			return nil, nil, err
		}
	}
	confirm := confirmWith(opt, v15, req.Stderr)
	if err := confirm(p); err != nil {
		return nil, nil, err
	}
	wopts := req.Build
	wopts.Mode = nbuild.ModeWrite
	wopts.Rand = newSeededRand(req.Seed)
	// Effects that change what the render reads (declared-plugin installs and
	// expired-plugin removals) are confirmed above and run here, inside the held
	// apply. The project is first checked unchanged, then the effects run, then
	// the plan of record is rendered again so the artifacts the plugins bring
	// are planned, shown and held like every other.
	pre := hasEffect(p, EffectPluginInstall) || hasEffect(p, EffectPluginRemove)
	if pre {
		if err := recheck(ctx, req, p.PlanID); err != nil {
			return nil, nil, err
		}
	}
	if opt.BeforeWrite != nil {
		if err := opt.BeforeWrite(); err != nil {
			return nil, nil, err
		}
	}
	if hasEffect(p, EffectPluginInstall) {
		if err := nbuild.InstallDeclaredPlugins(ctx, req.ProjectDir, req.Build.RemoteDeploy); err != nil {
			return nil, nil, err
		}
	}
	if pre {
		c, err = compute(ctx, req)
		if err != nil {
			return nil, nil, err
		}
		p = c.plan
		// P7-LIVE-04: a hand-edited file only the render after the plugin
		// changes overwrites gets its diff here too, before the second gate.
		if err := warnHandEdits(req, p, c, v15); err != nil {
			return nil, nil, err
		}
		if req.Stderr != nil && !p.Empty {
			_, _ = fmt.Fprintln(req.Stderr, "nself: after the plugin changes the build will write:")
			if err := RenderHuman(req.Stderr, *p); err != nil {
				return nil, nil, err
			}
		}
		// The render after the plugin changes is confirmed again: --yes accepts it
		// (it was printed above), a prompt asks again, and a hand-edited
		// overwrite still needs --force or a yes, interactive or not.
		if err := confirm(p); err != nil {
			return nil, nil, err
		}
	}
	if err := recheck(ctx, req, p.PlanID); err != nil {
		return nil, nil, err
	}
	if afterRecheck != nil {
		afterRecheck()
	}
	wopts.Expect = c.planned
	// A non-empty plan must be written: the freshness cache only compares .env
	// with docker-compose.yml and would skip changes in .env.secrets, nself.yaml,
	// plugin dirs and the like (EPIC round 3, F1).
	wopts.Force = wopts.Force || !p.Empty
	res, err := buildBuild(req.ProjectDir, wopts)
	if err != nil {
		return nil, nil, err
	}
	// P7-LIVE-04 (EPIC D5): the record is written only now that the whole
	// write succeeded, and atomically inside Save — an interrupted apply (a
	// failed write between two files) leaves the previous record intact, so
	// the next plan reports the half-written project as ordinary changes, not
	// hand-edits. The hashes are of the exact bytes the write was held to.
	firstRecorded := state == nil
	if state == nil {
		state = &GeneratedState{}
	}
	state.RecordPlanned(c.planned)
	if err := state.Save(req.ProjectDir); err != nil {
		return nil, nil, err
	}
	if firstRecorded && req.Stderr != nil {
		_, _ = fmt.Fprint(req.Stderr, firstRunNotice)
	}
	return p, res, nil
}

// confirmWith runs Confirm with stderr muted for the one case whose generic
// v1.4 notice would misname the flag: a hand-edit-only change (no prod-class
// confirmation due), where P7-LIVE-04's warning above already names --force.
func confirmWith(opt ApplyOptions, v15 bool, stderr io.Writer) func(*Plan) error {
	return func(p *Plan) error {
		if stderr != nil && !v15 && !p.RequiresConfirmation && len(HandEditedPaths(*p)) > 0 {
			stderr = nil
		}
		return Confirm(*p, opt, v15, stderr)
	}
}

// recheck renders again and refuses (E450) when the plan id moved.
func recheck(ctx context.Context, req Request, want string) error {
	again, err := compute(ctx, req)
	if err != nil {
		return err
	}
	if again.plan.PlanID != want {
		return errs.New("E450", "the project changed after the plan was shown").
			WithWhy("an input changed while the confirmation was pending; nothing was written").
			WithFix("re-run nself build --plan and confirm the new plan")
	}
	return nil
}

// hasEffect reports whether the plan carries an effect of kind k.
func hasEffect(p *Plan, k EffectKind) bool {
	for _, e := range p.Effects {
		if e.Kind == k {
			return true
		}
	}
	return false
}

// afterRecheck is a test seam run between the re-check and the write.
var afterRecheck func()

// short abbreviates an id for a message.
func short(id string) string {
	if len(id) > idPrefixLen {
		return id[:idPrefixLen]
	}
	return id
}

// seededRand is a SHA-256 counter-mode byte stream: the same seed yields the
// same bytes, and the stream is as strong as the seed (32 random bytes from
// crypto/rand in Apply).
type seededRand struct {
	seed []byte
	ctr  uint64
	buf  []byte
}

// newSeededRand returns a reader over the stream of seed.
func newSeededRand(seed []byte) io.Reader {
	return &seededRand{seed: append([]byte(nil), seed...)}
}

func (r *seededRand) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if len(r.buf) == 0 {
			var c [8]byte
			binary.BigEndian.PutUint64(c[:], r.ctr)
			r.ctr++
			sum := sha256.Sum256(append(append([]byte(nil), r.seed...), c[:]...))
			r.buf = sum[:]
		}
		k := copy(p[n:], r.buf)
		r.buf = r.buf[k:]
		n += k
	}
	return n, nil
}
