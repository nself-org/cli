package deploy

// Purpose: ApplyRemote, the v1.5 remote apply for one host (D24). The pipeline,
//          the legacy single-host deploy and (later) promote all call it.
// Inputs:  ApplyOptions: a Remote, the remote path, the staged ArtifactSet, the
//          snapshot the build used, the local plugin set, the tier flags and
//          the injected verified-snapshot func (controlplane owns it).
// Outputs: ApplyResult (backup id, host, pre-deploy tar, whether the sync was
//          reached), an error whose code says why it stopped.
// Constraints: step order, each later step unrun when one fails:
//   1 target snapshot + drift check (E485)   2 build-context check (E485)
//   3 deploy lock   4 pre-deploy tar (every tier)
//   5 verified database backup, prod-class only (E206): before any compose command
//   6 sync, no delete, env 0600, previous-shipped marker prune   7 compose model
//   8 nginx -t   9 save image ids, pull   10 recreate preview + stateful confirm (E403)
//   11 per service in compose order: up -d --no-deps, health gate (E250/E251)
//   12 final regression recheck, nginx reload   13 hasura metadata apply.
// A fronted project (Set.Front, D-0425) adds its fronting stack's nginx/sites as a
// second root (apply_remote_front.go): guards and container lookup before the lock,
// its own tar after step 4, its sync right after step 6, `docker exec <id> nginx -t`
// as step 8 and the reload in step 12; a failure before the first `up` restores both.
// A failure in steps 6-10 happens before any container is touched, so the
// pre-deploy tar is restored in place (files only, plus removal of what the
// deploy created), image tags the pull moved point back at their old images,
// and nothing is left half shipped. Later failures keep the new
// files and name the tar and `nself deploy rollback`.

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// ApplyOptions configures ApplyRemote.
type ApplyOptions struct {
	Remote     Remote
	RemotePath string
	// Env names the environment, for the rollback hint in errors.
	Env string
	Set ArtifactSet
	// Expected is the snapshot the build used; the host must still match it.
	// Nil skips that comparison (the local-set comparison still runs).
	Expected     *TargetSnapshot
	LocalPlugins []PluginRef
	// ProdClass: take the verified snapshot and require confirmation for a
	// stateful recreate.
	ProdClass bool
	Yes       bool
	Confirm   func(msg string) bool
	// BackupID is a verified database backup the caller already took for the whole
	// env (one per env, on its primary, before any server is touched). When set,
	// no backup is taken here.
	BackupID string
	// Previous are the paths the env's previous deploy shipped or pruned, from the
	// local record (deploy.json). They alone decide what the prune may remove; the
	// host's own list can only narrow them. Empty: nothing is pruned.
	Previous []string
	// Snapshot takes and verifies a database backup tagged with id and returns
	// its backup id (E206 on failure). Used when ProdClass and BackupID is empty.
	Snapshot func(ctx context.Context, id string) (string, error)
	// StrictHasura makes a failed metadata apply fail the deploy.
	StrictHasura bool
	Clock        docker.Clock
	Out          io.Writer
	Now          func() time.Time
	// KeepImages is set by a rollback (ReapplyRemote only): refs to point back at
	// the image ids the previous deploy ran, and not to pull again.
	KeepImages []imageTag
}

// ApplyResult records what ApplyRemote did, even when it stopped early.
type ApplyResult struct {
	BackupID     string // verified database backup (prod-class only)
	BackupHost   string
	PreDeployTar string
	// Synced is true once the artifact sync started and the host's files were not
	// put back: the deploy record must be written. A deploy that failed before
	// the first `up` and was restored in place reports false (the last good
	// record stays); if that restore itself failed it stays true.
	Synced bool
	// Changed are the paths this deploy shipped or pruned, Absent the subset the
	// host did not have before. Recorded locally; they drive a rollback.
	Changed     []string
	Absent      []string
	Recreated   []string
	FrontRecord // the fronting stack's tar and changed/absent lists (D-0425)
	// Touched is true once the first `up` ran: containers may have changed, so
	// the env's database backup belongs in the record.
	Touched bool
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
	if err := CheckBuildContexts(ctx, r, o.Set); err != nil {
		return res, err
	}

	fa, err := shipGuards(ctx, &o) // main and front root guards, E485 before the lock
	if err != nil {
		return res, err
	}
	id := NewBackupID(o.Now())
	release, err := AcquireLock(ctx, r, rp, id)
	if err != nil {
		return res, err
	}
	defer release()
	say("pre-deploy backup tar %s", id)
	bk, err := CreatePreDeployTar(ctx, r, rp, id, o.Set.shippedPaths())
	if err != nil {
		return res, err
	}
	res.BackupHost, res.PreDeployTar = r.Label(), bk.Tar
	res.Changed, res.Absent = o.Set.shippedPaths(), bk.Absent
	if err = fa.makeTar(ctx, o, id, &res); err != nil { // the front root's own tar, after the main one
		return res, err
	}
	switch {
	case o.ProdClass && o.BackupID != "":
		res.BackupID = o.BackupID
	case o.ProdClass && o.Snapshot == nil:
		return res, errs.New("E206", "no verified backup for a production target; nothing was applied")
	case o.ProdClass:
		say("verified database backup before any change")
		if res.BackupID, err = o.Snapshot(ctx, "pre-deploy-"+id); err != nil {
			return res, err
		}
	}

	// From the sync to the first `up`, a failure puts the old files back (the
	// restore survives a cancelled ctx) and removes what the deploy created.
	// Once the pull started it also points the moved image tags back (D-0426).
	touched := false
	var pulled []imageTag
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = fmt.Errorf("interrupted: %w", err)
		}
		if err != nil && !touched && len(pulled) > 0 {
			if terr := restoreImageTags(ctx, r, pulled); terr != nil {
				// Emitted as well: a rendered CLIError shows only its own message.
				warn := fmt.Sprintf("the image tags the pull moved could not be put back (%v): a manual `docker compose up` on the host would run the new images without the stateful-recreate confirmation", terr)
				emit(o.Out, "  [warn] %s", warn)
				err = fmt.Errorf("%w; %s", err, warn)
			} else {
				emit(o.Out, "  [restored] %d image tag(s) pointed back at the images they had before the pull", len(pulled))
			}
		}
		if err != nil && res.Synced && !touched {
			rerr := RestorePreDeployTar(ctx, r, rp, bk.Tar, RestoreSpec{Changed: res.Changed, Absent: res.Absent})
			if rerr = fa.restore(ctx, o, rerr); rerr != nil { // both roots, in place
				// The host may be half restored: keep the record, but never offer the
				// database backup for a deploy that changed no containers.
				res.BackupID = ""
				err = fmt.Errorf("%w; restoring %s also failed: %v", err, bk.Tar, rerr)
			} else {
				res.Synced = false // exact revert done: the last good record stays
			}
		} else if err != nil && touched {
			err = fmt.Errorf("%w; the new files are in place and the pre-deploy tar is %s: run 'nself deploy rollback %s'", err, bk.Tar, o.Env)
		}
	}()

	say("sync artifact set to %s:%s", r.Label(), rp)
	res.Synced = true
	if len(o.Previous) == 0 {
		say("no record of a previous deploy: nothing is pruned")
	}
	pruned, err := PrunePrevious(ctx, r, rp, o.Set.KeepSet(), o.Previous)
	if err != nil {
		return res, err
	}
	for _, f := range pruned {
		emit(o.Out, "  [pruned] %s (generated file the new set dropped)", f)
		res.Changed = append(res.Changed, f)
	}
	if err = SyncArtifacts(ctx, r, o.Set); err != nil {
		return res, err
	}
	if err = fa.sync(ctx, o, &res); err != nil { // front root right after the main sync
		return res, err
	}
	model, err := ReadComposeModel(ctx, r, rp, o.Set.Compose, o.Set.EnvManifest)
	if err != nil {
		return res, err
	}
	say("nginx -t in a throwaway container")
	if err = fa.checkNginx(ctx, o, model); err != nil {
		return res, err
	}
	base := composeBase(o.Set.Compose, o.Set.EnvManifest)
	if pulled, err = saveImageTags(ctx, r, imageRefs(model)); err != nil {
		return res, err
	}
	// The ids the pull is about to replace: a rollback points the tags back at them (D-0700).
	if err = writeImageRecord(ctx, r, bk.Tar, pulled); err != nil {
		return res, err
	}
	say("docker compose pull")
	if _, err = r.Run(ctx, shScript(fmt.Sprintf("cd %s && %s pull", q(rp), base))); err != nil {
		return res, fmt.Errorf("remote pull on %s: %w", r.Label(), err)
	}
	say("preview: docker compose up --dry-run")
	pv, err := PreviewRecreate(ctx, r, rp, o.Set.Compose, o.Set.EnvManifest, model)
	if err != nil {
		return res, err
	}
	res.Recreated = pv.Recreate
	emit(o.Out, "  [preview] recreate: %v, create: %v, stateful: %v", pv.Recreate, pv.Create, pv.Stateful)
	if len(pv.Unknown) > 0 && !o.ProdClass {
		emit(o.Out, "  [warn] preview lines not classified: %v", pv.Unknown)
	}
	if err = ConfirmStateful(pv, o.ProdClass, o.Yes, o.Confirm); err != nil {
		return res, err
	}
	touched, res.Touched = true, true
	if err = applyServices(ctx, o, model); err != nil {
		return res, err
	}
	if err = reloadNginx(ctx, o, model); err != nil {
		return res, err
	}
	if err = fa.reload(ctx, o.Out, r); err != nil {
		return res, err
	}
	if err = applyHasuraMetadata(ctx, o); err != nil {
		return res, err
	}
	// Fully successful: trim old pre-deploy tars, keeping this deploy's own (D-0421).
	pruneOldTars(ctx, o, bk.Tar)
	return res, nil
}

// ReapplyRemote runs the pull and the service-by-service up with the health
// gate over the manifests already on the host (rollback uses it after the files
// are back in place), then validates the restored nginx config in a throwaway
// container before reloading nginx, and applies the hasura metadata.
func ReapplyRemote(ctx context.Context, o ApplyOptions) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	model, err := ReadComposeModel(ctx, o.Remote, o.RemotePath, o.Set.Compose, o.Set.EnvManifest)
	if err != nil {
		return err
	}
	base := composeBase(o.Set.Compose, o.Set.EnvManifest)
	pull := fmt.Sprintf("cd %s && %s pull", q(o.RemotePath), base)
	if len(o.KeepImages) > 0 {
		// A rollback: the tags the deploy moved go back to the recorded ids and are
		// not pulled again; only the services without a record are (D-0700).
		if err = restoreImageTags(ctx, o.Remote, o.KeepImages); err != nil {
			return err
		}
		emit(o.Out, "  [restored] %d image tag(s) pointed back at the images the previous deploy ran", len(o.KeepImages))
		pull = ""
		if svcs := pullTargets(model, o.KeepImages); len(svcs) > 0 {
			pull = fmt.Sprintf("cd %s && %s pull -- %s", q(o.RemotePath), base, strings.Join(quoteAll(svcs), " "))
		}
	}
	if pull != "" {
		if _, err = o.Remote.Run(ctx, shScript(pull)); err != nil {
			return fmt.Errorf("remote pull on %s: %w", o.Remote.Label(), err)
		}
	}
	if err = applyServices(ctx, o, model); err != nil {
		return err
	}
	if err = CheckNginx(ctx, o.Remote, o.RemotePath, model, o.Out); err != nil {
		return fmt.Errorf("the restored nginx config does not pass nginx -t, nginx was not reloaded: %w", err)
	}
	if err = reloadNginx(ctx, o, model); err != nil {
		return err
	}
	return applyHasuraMetadata(ctx, o)
}

// shippedPaths are the set's relative paths plus the shipped list the sync writes.
// The host's own .env is never one of them.
func (s ArtifactSet) shippedPaths() []string {
	return append(append(append([]string{}, s.Files...), s.EnvFiles...), ShippedFile)
}
