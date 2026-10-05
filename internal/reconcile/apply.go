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

	c, err := compute(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	p := c.plan
	if opt.PlanID != "" {
		if c.generates {
			return nil, nil, errs.New("E451", "this build generates secrets, so --plan-id cannot bind it").
				WithWhy("random values cannot be reproduced from a plan id, so a plan id would not describe what is written").
				WithFix("set the secrets in .env.secrets first, or run nself build and confirm at the prompt (or with --yes, without --plan-id)")
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
	if err := Confirm(*p, opt, v15, req.Stderr); err != nil {
		return nil, nil, err
	}
	removing := false
	for _, e := range p.Effects {
		removing = removing || e.Kind == EffectPluginRemove
	}
	if opt.BeforeWrite != nil {
		if err := opt.BeforeWrite(); err != nil {
			return nil, nil, err
		}
	}
	wopts := req.Build
	wopts.Mode = nbuild.ModeWrite
	wopts.Rand = newSeededRand(req.Seed)
	if !removing {
		// Plugin removal changes the plugin dir the render read, so a plan with
		// it is known to differ from the write (documented); every other plan
		// must be unchanged, and the write is held to it.
		again, err := compute(ctx, req)
		if err != nil {
			return nil, nil, err
		}
		if again.plan.PlanID != p.PlanID {
			return nil, nil, errs.New("E450", "the project changed after the plan was shown").
				WithWhy("an input changed while the confirmation was pending; nothing was written").
				WithFix("re-run nself build --plan and confirm the new plan")
		}
		if afterRecheck != nil {
			afterRecheck()
		}
		wopts.Expect = c.planned
	}
	res, err := nbuild.Build(req.ProjectDir, wopts)
	if err != nil {
		return nil, nil, err
	}
	return p, res, nil
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
