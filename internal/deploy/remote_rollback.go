package deploy

// Purpose: `nself deploy rollback` for a v1.5 remote apply (D24): put the files
//          of the pre-deploy tar back in place, restore the verified database
//          backup when the deploy took one (production-class), then re-apply the
//          restored artifact set with the same pull, service-by-service up and
//          health gate.
// Inputs:  a Remote, the remote path, the tar path and backup id the deploy
//          recorded in .nself/state/deploy.json.
// Outputs: an error naming the step that failed.
// Constraints: the host deploy lock is held; files first, then the database,
//              then the re-apply, so hasura and the services see the old
//              configuration on the old schema (FilesOnly and ReapplyOnly split
//              the steps for a multi-host promotion). Nothing is moved or deleted but generated files
//              the tar does not hold. A host whose tar held no compose set (a
//              first deploy) is restored but not re-applied, and says so.

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/restoreconsent"
)

// RollbackOptions configures RollbackRemote.
type RollbackOptions struct {
	Remote     Remote
	RemotePath string
	Tar        string // PreDeployTar of the record
	BackupID   string // BackupID of the record; empty off prod-class
	// ProjectName goes to backup restore --confirm-project (C2-01); required with BackupID.
	ProjectName string
	// Changed and Absent come from the local deploy.json record of this host.
	Changed []string
	Absent  []string
	// DBRestored: the record says the database backup was already restored; it is
	// not restored again. OnDBRestored runs right after a successful restore,
	// before the re-apply, so the caller can persist it.
	DBRestored   bool
	OnDBRestored func() error
	// FrontTar, FrontChanged and FrontAbsent are the record of the fronting
	// stack's root (D-0425); all empty for a deploy that did not touch one.
	FrontTar     string
	FrontChanged []string
	FrontAbsent  []string
	StrictHasura bool
	Clock        docker.Clock
	Out          io.Writer
	// FilesOnly stops after the files (and a BackupID restore) are back: a
	// promotion restores every host's files, then its database once, then brings
	// each host up with ReapplyOnly (D-0699).
	FilesOnly bool
	// ReapplyOnly skips the file and database restore and re-applies the set the
	// host's restored tree names: the pull, the gated up, nginx and hasura.
	ReapplyOnly bool
}

// RollbackRemote undoes the last v1.5 remote apply on the host.
func RollbackRemote(ctx context.Context, o RollbackOptions) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	r, rp := o.Remote, o.RemotePath
	if err := restoreconsent.Precheck(o.BackupID, o.ProjectName, o.DBRestored); err != nil {
		return err
	}
	if _, err := ResolveDirs(ctx, r, rp, layoutDirs(o.Changed)); err != nil {
		return err
	}
	fb, err := prepareFrontRollback(ctx, o) // the fronting stack's guards and container, before the lock
	if err != nil {
		return err
	}
	release, err := AcquireLock(ctx, r, rp, strings.TrimSuffix(strings.TrimPrefix(path.Base(o.Tar), "pre-deploy-"), ".tar.gz"))
	if err != nil {
		return err
	}
	defer release()
	if !o.ReapplyOnly {
		emit(o.Out, "  [running] restore %s in place on %s", path.Base(o.Tar), r.Label())
		if err := RestorePreDeployTar(ctx, r, rp, o.Tar, RestoreSpec{Changed: o.Changed, Absent: o.Absent}); err != nil {
			return err
		}
		if err := fb.restore(ctx, o); err != nil {
			return err
		}
	}
	set, restored, err := restoredSet(ctx, o)
	if err != nil {
		return err
	}
	var keep []imageTag
	if restored {
		// Before the database restore: a recorded image that is gone refuses the
		// rollback here, with the containers and the database untouched (D-0700).
		model, merr := ReadComposeModel(ctx, r, rp, set.Compose, set.EnvManifest)
		if merr != nil {
			// With a record, a pull on a later retry would bring the new image back: stop
			// before the database restore. Without one the re-apply reports it, as it always did.
			if saved, found, rerr := readImageRecord(ctx, r, o.Tar); rerr == nil && found && len(saved) > 0 {
				return fmt.Errorf("rollback stopped before the database restore: the restored compose cannot be resolved to check the recorded images (the files are restored): %w", merr)
			}
		} else if keep, err = imagesToRestore(ctx, r, o.Tar, model); err != nil {
			return err
		}
	}
	switch {
	case o.ReapplyOnly:
	case o.BackupID != "" && o.DBRestored:
		emit(o.Out, "  [skip] database backup %s was already restored by an earlier rollback; not restoring it again", o.BackupID)
	case o.BackupID != "":
		if err := restoreconsent.RunRestore(ctx, r, rp, o.BackupID, o.ProjectName, o.Out, o.OnDBRestored); err != nil {
			return err
		}
	}
	if o.FilesOnly {
		emit(o.Out, "  [done] files restored; the restored set is brought up after the database restore")
		return nil
	}
	// The front files are back: the live fronting nginx is tested and reloaded even
	// when the re-apply failed, else it keeps serving the rolled-back confs.
	aerr := reapplyRestored(ctx, o, set, restored, keep)
	ferr := fb.reload(ctx, o)
	switch {
	case aerr != nil && ferr != nil:
		return fmt.Errorf("%w; the fronting nginx reload also failed: %v", aerr, ferr)
	case aerr != nil && fb != nil:
		return fmt.Errorf("%w; the fronting nginx was tested and reloaded with the restored files", aerr)
	case aerr != nil:
		return aerr
	}
	return ferr
}

// restoredSet reads the compose set the restored tree names. restored is false
// for a host whose tar held no compose file (a first deploy): nothing to re-apply.
func restoredSet(ctx context.Context, o RollbackOptions) (set ArtifactSet, restored bool, err error) {
	return hostComposeSet(ctx, o.Remote, o.RemotePath)
}

// hostComposeSet reads the compose set the host's manifests under rp name;
// found is false when the tree holds no compose file.
func hostComposeSet(ctx context.Context, r Remote, rp string) (set ArtifactSet, found bool, err error) {
	compose, err := remoteLines(ctx, r, path.Join(rp, ".nself", "compose-files.txt"))
	if err != nil {
		return set, false, err
	}
	envs, err := remoteLines(ctx, r, path.Join(rp, ".nself", "compose-env-files.txt"))
	if err != nil {
		return set, false, err
	}
	if len(compose) == 0 {
		// A host last deployed by v1.4 has no manifest, only docker-compose.yml.
		single, err := remoteLines(ctx, r, path.Join(rp, "docker-compose.yml"))
		if err != nil {
			return set, false, err
		}
		if len(single) == 0 {
			return set, false, nil
		}
		compose = []string{path.Join(rp, "docker-compose.yml")}
		if e, _ := remoteLines(ctx, r, path.Join(rp, ".env")); len(e) > 0 {
			envs = []string{path.Join(rp, ".env")}
		}
	}
	return ArtifactSet{Compose: compose, EnvManifest: envs}, true, nil
}

// reapplyRestored re-applies the restored artifact set: the pull (skipped for
// the images keep puts back), the services in compose order with the health
// gate, nginx and hasura. A host whose tar held no compose set (a first deploy)
// is restored but not re-applied.
func reapplyRestored(ctx context.Context, o RollbackOptions, set ArtifactSet, restored bool, keep []imageTag) error {
	if !restored {
		emit(o.Out, "  [skip] the restored tree has no compose file (it was a first deploy): files restored, nothing re-applied")
		return nil
	}
	emit(o.Out, "  [running] re-apply the restored artifact set")
	return ReapplyRemote(ctx, ApplyOptions{
		Remote: o.Remote, RemotePath: o.RemotePath, Set: set, StrictHasura: o.StrictHasura, Clock: o.Clock, Out: o.Out, KeepImages: keep,
	})
}

// remoteLines reads a text file of one path per line; a missing file is empty.
func remoteLines(ctx context.Context, r Remote, file string) ([]string, error) {
	out, err := r.Run(ctx, shScript(fmt.Sprintf(`if [ -f %s ]; then cat %s; fi`, q(file), q(file))))
	if err != nil {
		return nil, fmt.Errorf("reading %s on %s: %w", file, r.Label(), err)
	}
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}
