package deploy

// Purpose: `nself deploy rollback` for a v1.5 remote apply (D24): put the files
//          of the pre-deploy tar back in place, restore the verified database
//          backup when the deploy took one (production-class), then re-apply the
//          restored artifact set with the same pull, service-by-service up and
//          health gate.
// Inputs:  a Remote, the remote path, the tar path and backup id the deploy
//          recorded in .nself/state/deploy.json.
// Outputs: an error naming the step that failed.
// Constraints: files first, so the database restore and the re-apply see the old
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
	Clock      docker.Clock
	Out        io.Writer
}

// RollbackRemote undoes the last v1.5 remote apply on the host.
func RollbackRemote(ctx context.Context, o RollbackOptions) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	r, rp := o.Remote, o.RemotePath
	emit(o.Out, "  [running] restore %s in place on %s", path.Base(o.Tar), r.Label())
	pruned, err := RestorePreDeployTar(ctx, r, rp, o.Tar)
	if err != nil {
		return err
	}
	for _, f := range pruned {
		emit(o.Out, "  [pruned] %s (generated file the old set did not have)", f)
	}
	if o.BackupID != "" {
		emit(o.Out, "  [running] restore database backup %s", o.BackupID)
		if _, err := r.Run(ctx, fmt.Sprintf("cd %s && nself backup restore %s --yes", q(rp), q(o.BackupID))); err != nil {
			return fmt.Errorf("restoring backup %s on %s: %w; the files are restored, the stack was not restarted", o.BackupID, r.Label(), err)
		}
	}
	compose, err := remoteLines(ctx, r, path.Join(rp, ".nself", "compose-files.txt"))
	if err != nil {
		return err
	}
	if len(compose) == 0 {
		emit(o.Out, "  [skip] the restored tree has no compose manifest (it was a first deploy): files restored, nothing re-applied")
		return nil
	}
	envs, err := remoteLines(ctx, r, path.Join(rp, ".nself", "compose-env-files.txt"))
	if err != nil {
		return err
	}
	emit(o.Out, "  [running] re-apply the restored artifact set")
	return ReapplyRemote(ctx, ApplyOptions{
		Remote: r, RemotePath: rp, Set: ArtifactSet{Compose: compose, EnvManifest: envs}, Clock: o.Clock, Out: o.Out,
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
