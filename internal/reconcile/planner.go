package reconcile

// planner.go — reconcile.Compute: the read-only half of the change path
// (EPIC P7-LIVE D6).
//
// Purpose: say what `nself build` would change, exactly, without changing it.
// Compute runs the real build pipeline in plan mode (P7-LIVE-21), diffs what it
// would write against the disk, adds the host effects it recorded and the
// container impact, and returns one finalized change plan. Apply runs the same
// function, so what was shown is what is applied.
// Inputs: a Request. Outputs: a finalized *Plan (contract:cli.change-plan v1).
// Constraints: writes nothing under the project, ~/.nself, the fronting stack
// or the host. A failure of the build, a disk read or the finalization is
// returned, never replaced by an empty plan.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime"
	"sort"
	"strings"

	nbuild "github.com/nself-org/cli/internal/build"
)

// Compute computes the change plan of req. (EPIC D6 names it reconcile.Plan; that
// name is the Plan type of P7-LIVE-01, so the function is Compute.)
func Compute(ctx context.Context, req Request) (*Plan, error) {
	c, err := compute(ctx, req)
	if err != nil {
		return nil, err
	}
	return c.plan, nil
}

// computed is one render and everything derived from it: the plan, the planned
// build (the exact bytes, modes and removals Apply will hold the write to) and
// the before/after artifact sets.
type computed struct {
	plan          *Plan
	planned       *nbuild.PlannedBuild
	before, after ArtifactSet
	generates     bool   // the render generates secrets (random values)
	record        []byte // the exact record bytes this plan's apply writes (P7-LIVE-04)
}

// compute renders once in plan mode and builds the plan from that render.
func compute(ctx context.Context, req Request) (*computed, error) {
	opts := req.Build
	if opts.Check {
		return nil, fmt.Errorf("a plan cannot be computed for --check: --check validates and writes nothing")
	}
	opts.Mode = nbuild.ModePlan
	opts.Expect = nil
	if len(req.Seed) > 0 {
		opts.Rand = newSeededRand(req.Seed)
	}
	res, err := nbuild.Build(req.ProjectDir, opts)
	if err != nil {
		return nil, err
	}
	if res == nil || res.Planned == nil {
		return nil, fmt.Errorf("plan-mode build returned no planned artifacts")
	}
	ov := newOverlay(req.ProjectDir, res.FrontingDir, res.Planned)
	before, after, err := artifactSets(ov, res.Planned)
	if err != nil {
		return nil, err
	}
	// P7-LIVE-04 (EPIC D5): the generated-state record is itself a planned
	// artifact, so plan == apply stays exact — the record the apply writes is
	// the one the plan reported, with the bytes shown here. Its planned bytes
	// are the current record with this render folded in; folding is idempotent,
	// so a plan after a successful apply does not list the record again. The
	// record is never hand_edited: it holds no entry for itself, so there is no
	// recorded hash a human edit could diverge from (an edit shows as an
	// ordinary change with its diff).
	state, raw, err := loadGeneratedStateRaw(req.ProjectDir)
	if err != nil {
		return nil, err
	}
	_, recBody, err := PlannedRecord(state, res.Planned)
	if err != nil {
		return nil, err
	}
	after[GeneratedStateDisplayPath] = File{Data: recBody, Perm: 0o644}
	if state != nil {
		perm := fs.FileMode(0o644)
		if info, serr := os.Stat(GeneratedStatePath(req.ProjectDir)); serr == nil {
			perm = info.Mode().Perm()
		}
		before[GeneratedStateDisplayPath] = File{Data: raw, Perm: perm}
	}
	arts := append(Diff(before, after, req.HandEdited), modeArtifacts(before, after)...)
	p := &Plan{
		Command:   req.Command,
		Trigger:   req.Trigger,
		Env:       res.Env,
		Artifacts: arts,
		Effects:   append(effectsOf(res.Planned.Effects, hasSitesChange(arts)), req.Extra...),
	}
	if p.Command == "" {
		p.Command = CmdBuild
	}
	if p.Trigger.Kind == "" {
		p.Trigger.Kind = TriggerBuild
	}
	p.Containers = Containers{Known: true, Items: []ContainerItem{}}
	// Container impact asks Docker, so a dev build that needs no plan output
	// skips it; a prod-class plan, --plan and --json (req.Containers) always ask.
	if req.Containers || EnvClass(NormalizeEnv(res.Env)) == ClassProd {
		c, extra := containerImpact(ctx, req, req.ProjectDir, res, ov)
		p.Containers = c
		p.Effects = append(p.Effects, extra...)
	}
	if err := p.Finalize(); err != nil {
		return nil, err
	}
	id, err := contentPlanID(*p, after)
	if err != nil {
		return nil, err
	}
	p.PlanID = id
	if req.DiffOut != nil {
		if err := writeDiffs(req.DiffOut, p.Artifacts, before, after); err != nil {
			return nil, err
		}
	}
	gen := false
	for _, e := range p.Effects {
		gen = gen || e.Kind == EffectSecretsPersist
	}
	return &computed{plan: p, planned: res.Planned, before: before, after: after, generates: gen, record: recBody}, nil
}

// modeArtifacts lists files whose bytes do not change but whose permission bits
// would (EPIC ruling C): a change with diff_lines 0. Not reported on Windows,
// where permission bits are not meaningful.
func modeArtifacts(before, after ArtifactSet) []Artifact {
	if runtime.GOOS == "windows" {
		return nil
	}
	var out []Artifact
	for path, a := range after {
		b, ok := before[path]
		if !ok || !bytes.Equal(a.Data, b.Data) || a.Perm == b.Perm {
			continue
		}
		out = append(out, Artifact{Kind: Kind(path), Path: path, Action: ActionChange, Generated: IsGenerated(a.Data),
			DiffLines: 0, Redacted: IsEnvPath(path)})
	}
	return out
}

// writeDiffs writes the unified diff of every changed artifact, in plan order.
func writeDiffs(w io.Writer, arts []Artifact, before, after ArtifactSet) error {
	for _, a := range arts {
		if _, err := io.WriteString(w, Unified(a.Path, before[a.Path].Data, after[a.Path].Data)); err != nil {
			return err
		}
	}
	return nil
}

// artifactSets builds the before and after sets over every path the planned
// build writes, removes or chmods, keyed by display path: what is on disk now,
// and what the build would leave. Files the build does not touch are in neither.
// Plugin compose files rewritten in place and any other file outside the project
// appear as "@plugins/<rel>" and "@abs/<path>", so the plan and its id cover them.
func artifactSets(ov overlay, pb *nbuild.PlannedBuild) (before, after ArtifactSet, err error) {
	before, after = ArtifactSet{}, ArtifactSet{}
	seen := map[string]bool{}
	var keys []string
	add := func(k string) {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for k := range pb.Files {
		add(k)
	}
	for _, k := range pb.Removed {
		add(k)
	}
	for k := range pb.Modes {
		add(k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		name := ov.display(k)
		disk := ov.diskPath(k)
		info, statErr := os.Stat(disk)
		switch {
		case statErr == nil && info.Mode().IsRegular():
			data, rerr := os.ReadFile(disk)
			if rerr != nil {
				return nil, nil, fmt.Errorf("reading %s for the plan: %w", name, rerr)
			}
			before[name] = File{Data: data, Perm: info.Mode().Perm()}
		case statErr != nil && !os.IsNotExist(statErr):
			return nil, nil, fmt.Errorf("reading %s for the plan: %w", name, statErr)
		}
		if f, ok := pb.Files[k]; ok {
			after[name] = File{Data: f.Data, Perm: f.Perm}
		} else if m, ok := pb.Modes[k]; ok {
			if b, had := before[name]; had {
				after[name] = File{Data: b.Data, Perm: m}
			}
		}
	}
	return before, after, nil
}

// effectsOf converts recorded build effects. Two are bookkeeping, not changes,
// and are dropped: the build lock (how the build excludes other commands) and
// the nginx/sites snapshot when the build leaves nginx/sites alone (every
// build takes the snapshot, so listing it would make a plan that changes
// nothing look non-empty; it matters only when the build rewrites or removes a
// site conf, which sitesChanged reports).
func effectsOf(in []nbuild.PlannedEffect, sitesChanged bool) []Effect {
	out := []Effect{}
	for _, e := range in {
		if e.Kind == nbuild.EffectBuildLock || (e.Kind == nbuild.EffectNginxSitesBackup && !sitesChanged) {
			continue
		}
		out = append(out, Effect{Kind: EffectKind(e.Kind), Target: e.Target, Detail: e.Detail})
	}
	return out
}

// hasSitesChange reports whether the plan changes or removes a file under an
// nginx/sites directory (the project's or the fronting stack's).
func hasSitesChange(arts []Artifact) bool {
	for _, a := range arts {
		if a.Kind == KindNginx && (a.Action != ActionAdd) && strings.Contains(a.Path, "nginx/sites/") {
			return true
		}
	}
	return false
}

// contentPlanID is the plan_id of EPIC D9 as amended 2026-10-05: sha256 over
// the canonical plan JSON, then, for every artifact in path order, its path, the
// sha256 of the bytes it would leave (a removal hashes as "removed") and its
// permission bits. The id so binds the content of the change, not only its
// shape; the bytes themselves never reach the plan JSON. A render that
// generates secrets hashes those random values too, so its id is only
// reproducible with the same seed: Apply refuses --plan-id for such a render
// (E451) and binds it through its own single render instead.
func contentPlanID(p Plan, after ArtifactSet) (string, error) {
	canon, err := CanonicalJSON(p)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write(canon)
	for _, a := range p.Artifacts { // Finalize sorted these by path
		sum, perm := "removed", ""
		if f, ok := after[a.Path]; ok {
			d := sha256.Sum256(f.Data)
			sum, perm = hex.EncodeToString(d[:]), fmt.Sprintf("%04o", f.Perm)
		}
		_, _ = fmt.Fprintf(h, "\n%s\x00%s\x00%s", a.Path, sum, perm)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
