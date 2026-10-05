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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	nbuild "github.com/nself-org/cli/internal/build"
)

// Compute computes the change plan of req. (EPIC D6 names it reconcile.Plan; that
// name is the Plan type of P7-LIVE-01, so the function is Compute.)
func Compute(ctx context.Context, req Request) (*Plan, error) {
	opts := req.Build
	if opts.Check {
		return nil, fmt.Errorf("a plan cannot be computed for --check: --check validates and writes nothing")
	}
	opts.Mode = nbuild.ModePlan
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
	arts := Diff(before, after, req.HandEdited)
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
	if req.Containers {
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
	return p, nil
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

// artifactSets builds the before and after sets over the paths the planned
// build writes or removes: before is what is on disk now, after what the build
// would leave. Files the build does not touch appear in neither.
func artifactSets(ov overlay, pb *nbuild.PlannedBuild) (before, after ArtifactSet, err error) {
	before, after = ArtifactSet{}, ArtifactSet{}
	keys := make([]string, 0, len(pb.Files)+len(pb.Removed))
	for k := range pb.Files {
		keys = append(keys, k)
	}
	keys = append(keys, pb.Removed...)
	sort.Strings(keys)
	for _, k := range keys {
		if !ownedKey(k) {
			continue
		}
		info, statErr := os.Stat(ov.diskPath(k))
		switch {
		case statErr == nil && info.Mode().IsRegular():
			data, rerr := os.ReadFile(ov.diskPath(k))
			if rerr != nil {
				return nil, nil, fmt.Errorf("reading %s for the plan: %w", k, rerr)
			}
			before[k] = File{Data: data, Perm: info.Mode().Perm()}
		case statErr != nil && !os.IsNotExist(statErr):
			return nil, nil, fmt.Errorf("reading %s for the plan: %w", k, statErr)
		}
		if f, ok := pb.Files[k]; ok {
			after[k] = File{Data: f.Data, Perm: f.Perm}
		}
	}
	return before, after, nil
}

// ownedKey reports whether a planned key is an artifact of the plan: inside the
// project or the fronting stack. A key outside both (an absolute path, such as
// a plugin fragment rewritten in place under the plugin directory) is reported
// by the plugin-fragment effect instead of as a project artifact.
func ownedKey(k string) bool {
	return len(k) > 0 && k[0] != '/' && (len(k) < 2 || k[1] != ':')
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
// the canonical plan JSON, then, for every artifact in path order, its path and
// the sha256 of the bytes it would leave (a removal hashes as "removed"). The
// id so binds the content of the change, not only its shape; the bytes
// themselves never reach the plan JSON.
func contentPlanID(p Plan, after ArtifactSet) (string, error) {
	canon, err := CanonicalJSON(p)
	if err != nil {
		return "", err
	}
	// A run that generates secrets (secrets-persist) renders them into env-kind
	// artifacts with values that differ per run, so those artifacts cannot be
	// bound by content; their hash is the constant "generated". Once the
	// secrets are persisted the next plan binds them like any other file.
	generating := false
	for _, e := range p.Effects {
		generating = generating || e.Kind == EffectSecretsPersist
	}
	h := sha256.New()
	_, _ = h.Write(canon)
	for _, a := range p.Artifacts { // Finalize sorted these by path
		sum := "removed"
		if f, ok := after[a.Path]; ok {
			d := sha256.Sum256(f.Data)
			sum = hex.EncodeToString(d[:])
		}
		if generating && a.Kind == KindEnv {
			sum = "generated"
		}
		_, _ = fmt.Fprintf(h, "\n%s\x00%s", a.Path, sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
