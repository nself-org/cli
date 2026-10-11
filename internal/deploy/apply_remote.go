package deploy

// Purpose: ApplyRemote, the v1.5 remote apply for one host (D24). The pipeline,
//          the legacy single-host deploy and (later) promote all call it.
// Inputs:  ApplyOptions: a Remote, the remote path, the staged ArtifactSet, the
//          snapshot the build used, the local plugin set, the tier flags and
//          the injected verified-snapshot func (controlplane owns it).
// Outputs: ApplyResult (backup id, host, pre-deploy tar), an error whose code
//          says why it stopped.
// Constraints: step order, each later step unrun when one fails:
//   1 target snapshot + drift check (E485)   2 pre-deploy tar (every tier)
//   3 sync, no delete, env 0600, marker-only prune   4 compose model
//   5 nginx -t   6 recreate preview + stateful confirm (E403)
//   7 verified snapshot, prod-class only (E206)   8 pull
//   9 per service in compose order: up -d --no-deps, health gate (E250/E251)
//   10 final regression recheck   11 hasura metadata apply (legacy command).
// A failure in steps 3-8 happens before any container is touched, so the
// pre-deploy tar is restored in place (files only) and nothing is left half
// shipped. Later failures keep the new files and report the tar for
// `nself deploy rollback`.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// ApplyOptions configures ApplyRemote.
type ApplyOptions struct {
	Remote     Remote
	RemotePath string
	Set        ArtifactSet
	// Expected is the snapshot the build used; the host must still match it.
	// Nil skips that comparison (the local-set comparison still runs).
	Expected     *TargetSnapshot
	LocalPlugins []PluginRef
	// ProdClass: take the verified snapshot and require confirmation for a
	// stateful recreate.
	ProdClass bool
	Yes       bool
	Confirm   func(msg string) bool
	// Snapshot takes and verifies a database backup tagged with id and returns
	// its backup id (E206 on failure). Required when ProdClass.
	Snapshot func(ctx context.Context, id string) (string, error)
	// StrictHasura makes a failed metadata apply fail the deploy.
	StrictHasura bool
	Clock        docker.Clock
	Out          io.Writer
	Now          func() time.Time
}

// ApplyResult records what ApplyRemote did, even when it stopped early.
type ApplyResult struct {
	BackupID     string // verified database backup (prod-class only)
	BackupHost   string
	PreDeployTar string
	Recreated    []string
}

func emit(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format+"\n", a...) }

// ApplyRemote applies o.Set to the host.
func ApplyRemote(ctx context.Context, o ApplyOptions) (res ApplyResult, err error) {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	r, rp := o.Remote, o.RemotePath
	say := func(format string, a ...any) { emit(o.Out, "  [running] "+format, a...) }

	say("target snapshot on %s", r.Label())
	snap, err := TakeTargetSnapshot(ctx, r, rp)
	if err != nil {
		return res, err
	}
	diffs := PluginDrift(o.LocalPlugins, snap)
	if o.Expected != nil {
		diffs = LayoutDrift(*o.Expected, snap)
	}
	if err := DriftError(diffs); err != nil {
		return res, err
	}

	id := NewBackupID(o.Now())
	say("pre-deploy backup tar %s", id)
	bk, err := CreatePreDeployTar(ctx, r, rp, id)
	if err != nil {
		return res, err
	}
	res.BackupHost, res.PreDeployTar = r.Label(), bk.Tar

	// From here to the first `up`, a failure puts the old files back.
	touched := false
	defer func() {
		if err != nil && !touched {
			if _, rerr := RestorePreDeployTar(ctx, r, rp, bk.Tar); rerr != nil {
				err = fmt.Errorf("%w; restoring %s also failed: %v", err, bk.Tar, rerr)
			}
		}
	}()

	say("sync artifact set to %s:%s", r.Label(), rp)
	if err = SyncArtifacts(ctx, r, o.Set); err != nil {
		return res, err
	}
	removed, err := PruneMarked(ctx, r, rp, o.Set.KeepSet())
	if err != nil {
		return res, err
	}
	for _, f := range removed {
		emit(o.Out, "  [pruned] %s (generated file the new set dropped)", f)
	}
	model, err := ReadComposeModel(ctx, r, rp, o.Set.Compose, o.Set.EnvManifest)
	if err != nil {
		return res, err
	}
	say("nginx -t in a throwaway container")
	if err = CheckNginx(ctx, r, rp, model, o.Out); err != nil {
		return res, err
	}
	say("preview: docker compose up --dry-run")
	prev, err := PreviewRecreate(ctx, r, rp, o.Set.Compose, o.Set.EnvManifest, model)
	if err != nil {
		return res, err
	}
	res.Recreated = prev.Recreate
	emit(o.Out, "  [preview] recreate: %v, create: %v, stateful: %v", prev.Recreate, prev.Create, prev.Stateful)
	if err = ConfirmStateful(prev, o.ProdClass, o.Yes, o.Confirm); err != nil {
		return res, err
	}
	if o.ProdClass {
		if o.Snapshot == nil {
			return res, errs.New("E206", "no verified backup function for a production target; nothing was applied")
		}
		say("verified database backup before the apply")
		if res.BackupID, err = o.Snapshot(ctx, "pre-deploy-"+id); err != nil {
			return res, err
		}
	}
	base := composeBase(o.Set.Compose, o.Set.EnvManifest)
	say("docker compose pull")
	if _, err = r.Run(ctx, shScript(fmt.Sprintf("cd %s && %s pull", q(rp), base))); err != nil {
		return res, fmt.Errorf("remote pull on %s: %w", r.Label(), err)
	}
	touched = true
	if err = applyServices(ctx, o, model); err != nil {
		return res, err
	}
	return res, applyHasuraMetadata(ctx, o)
}

// ReapplyRemote runs the pull and the service-by-service up with the health
// gate over the manifests already on the host (rollback uses it after the
// files are back in place).
func ReapplyRemote(ctx context.Context, o ApplyOptions) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	model, err := ReadComposeModel(ctx, o.Remote, o.RemotePath, o.Set.Compose, o.Set.EnvManifest)
	if err != nil {
		return err
	}
	base := composeBase(o.Set.Compose, o.Set.EnvManifest)
	if _, err = o.Remote.Run(ctx, shScript(fmt.Sprintf("cd %s && %s pull", q(o.RemotePath), base))); err != nil {
		return fmt.Errorf("remote pull on %s: %w", o.Remote.Label(), err)
	}
	return applyServices(ctx, o, model)
}

// applyServices brings each service up in compose order and gates it, then
// re-reads every gated service once.
func applyServices(ctx context.Context, o ApplyOptions, model ComposeModel) error {
	r, rp, files, envs := o.Remote, o.RemotePath, o.Set.Compose, o.Set.EnvManifest
	type gated struct {
		name string
		seen []docker.ContainerState
	}
	var done []gated
	for _, svc := range model.Order {
		spec, _ := model.Specs(svc)
		marks, err := RemoteStartMarks(ctx, r, rp, files, envs, svc)
		if err != nil {
			return gateError(svc, fmt.Errorf("%w: %w", docker.ErrStateRead, err))
		}
		emit(o.Out, "  [running] Restart %s (sequenced rolling)", svc)
		up := fmt.Sprintf("cd %s && %s up -d --no-deps %s", q(rp), composeBase(files, envs), q(svc))
		if _, err := r.Run(ctx, shScript(up)); err != nil {
			return fmt.Errorf("remote restart of %s failed: %w\nRun 'nself logs %s' on the host for details", svc, err, svc)
		}
		var seen []docker.ContainerState
		states := func(c context.Context) ([]docker.ContainerState, error) {
			cs, err := RemoteStates(c, r, rp, files, envs, svc, marks)
			if err == nil {
				seen = cs
			}
			return cs, err
		}
		if err := docker.HealthGate(ctx, spec, states, o.Clock); err != nil {
			return gateError(svc, err)
		}
		done = append(done, gated{svc, seen})
	}
	for _, g := range done {
		cs, err := RemoteStates(ctx, r, rp, files, envs, g.name, nil)
		if err == nil {
			err = docker.Regressed(g.seen, cs)
		} else {
			err = fmt.Errorf("%w: %w", docker.ErrStateRead, err)
		}
		if err != nil {
			return gateError(g.name, err)
		}
	}
	return nil
}

// gateError classes a gate failure like the local rolling restart: E250 when
// the service is broken, E251 when it was too slow, no code when docker could
// not be read or the user interrupted.
func gateError(svc string, err error) error {
	hint := fmt.Sprintf("Run 'nself logs %s' on the host for details", svc)
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("remote apply interrupted at service %s: %w", svc, err)
	case errors.Is(err, docker.ErrStateRead):
		return fmt.Errorf("remote apply: cannot read the state of service %s, so it was not judged: %w", svc, err)
	case errors.Is(err, docker.ErrOneShotFailed), errors.Is(err, docker.ErrServiceCrashed):
		return errs.Wrap("E250", fmt.Sprintf("remote apply: %v. %s", err, hint), err)
	case errors.Is(err, docker.ErrGateTimeout):
		return errs.Wrap("E251", fmt.Sprintf("remote apply: %v. %s", err, hint), err)
	}
	return fmt.Errorf("remote apply: service %s: %w", svc, err)
}

// applyHasuraMetadata runs the legacy metadata apply on the host (cleanly a
// no-op when hasura/ is absent): strict fails the deploy, otherwise a warning.
func applyHasuraMetadata(ctx context.Context, o ApplyOptions) error {
	emit(o.Out, "  [running] hasura metadata apply on %s", o.Remote.Label())
	cmd := fmt.Sprintf("cd %s && for i in 1 2 3 4 5; do nself db hasura metadata apply && exit 0; sleep 3; done; exit 1", q(o.RemotePath))
	if out, err := o.Remote.Run(ctx, cmd); err != nil {
		msg := fmt.Sprintf("remote hasura metadata apply failed: %v\n%s", err, string(out))
		if o.StrictHasura {
			return fmt.Errorf("%s", msg)
		}
		emit(o.Out, "  [warn] %s (non-fatal; set NSELF_HASURA_METADATA_STRICT=true to fail deploys on this)", msg)
	}
	return nil
}
