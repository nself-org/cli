package deploy

// Purpose: `nself deploy rollback` for a v1.5 remote apply (D24): put the files
//          of the pre-deploy tar back in place, restore the verified database
//          backup when the deploy took one (production-class), then re-apply the
//          restored artifact set with the same pull, service-by-service up and
//          health gate.
// Inputs:  a Remote, the remote path, the tar path and backup id the deploy
//          recorded in .nself/state/deploy.json.
// Outputs: an error naming the step that failed.
// Constraints: the host deploy lock is held; files first, so the database restore and the re-apply see the old
//              configuration. Nothing is moved or deleted but generated files
//              the tar does not hold. A host whose tar held no compose set (a
//              first deploy) is restored but not re-applied, and says so.

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// RollbackOptions configures RollbackRemote.
type RollbackOptions struct {
	Remote     Remote
	RemotePath string
	Tar        string // PreDeployTar of the record
	BackupID   string // BackupID of the record; empty off prod-class
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
}

// RollbackRemote undoes the last v1.5 remote apply on the host.
func RollbackRemote(ctx context.Context, o RollbackOptions) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	r, rp := o.Remote, o.RemotePath
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
	emit(o.Out, "  [running] restore %s in place on %s", path.Base(o.Tar), r.Label())
	if err := RestorePreDeployTar(ctx, r, rp, o.Tar, RestoreSpec{Changed: o.Changed, Absent: o.Absent}); err != nil {
		return err
	}
	if err := fb.restore(ctx, o); err != nil {
		return err
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
	case o.BackupID != "" && o.DBRestored:
		emit(o.Out, "  [skip] database backup %s was already restored by an earlier rollback; not restoring it again", o.BackupID)
	case o.BackupID != "":
		emit(o.Out, "  [running] restore database backup %s", o.BackupID)
		if _, err := r.Run(ctx, shScript(fmt.Sprintf("cd %s && nself backup restore %s --yes", q(rp), q(o.BackupID)))); err != nil {
			return fmt.Errorf("restoring backup %s on %s: %w; the files are restored, the stack was not restarted", o.BackupID, r.Label(), err)
		}
		if o.OnDBRestored != nil {
			if err := o.OnDBRestored(); err != nil {
				return fmt.Errorf("the database backup was restored but the record could not be saved (do NOT run the rollback again): %w", err)
			}
		}
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
	r, rp := o.Remote, o.RemotePath
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
