package promote

// Purpose: Shared remote-only promotion steps used by the CLI and simulation.
// Inputs: A deploy.Remote and its validated project path.
// Outputs: Engine Steps that health-check, snapshot, save, migrate and restore.
// Constraints: Commands use validated paths and shell-quoted values; this package
// never builds on a host and the save step performs plain file reads only.

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

var snapshotTagRE = regexp.MustCompile(`^[a-z0-9-]+$`)

// SavedManifest is the target's compose and env-file manifest before apply.
type SavedManifest struct {
	Host         string   `json:"host"`
	ComposeFiles []string `json:"compose_files,omitempty"`
	EnvFiles     []string `json:"env_files,omitempty"`
}

// SourceHealthStep checks the source primary without changing it.
func SourceHealthStep(r deploy.Remote, remotePath string) Step {
	return commandStep("source-health", r, remotePath, "nself doctor")
}

// HealthStep checks the target primary and is reused after rollback.
func HealthStep(r deploy.Remote, remotePath string) Step {
	return commandStep("health", r, remotePath, "nself doctor")
}

// MigrateStep runs target migrations. Engine marks MigrateStarted first.
func MigrateStep(r deploy.Remote, remotePath string) Step {
	return commandStep("migrate", r, remotePath, "nself db migrate up")
}

func commandStep(name string, r deploy.Remote, remotePath, command string) Step {
	return Step{Name: name, Run: func(ctx context.Context, _ *State) error {
		if err := deploy.ValidateRemotePath(remotePath); err != nil {
			return err
		}
		_, err := r.Run(ctx, "cd "+remotesdk.ShellQuote(remotePath)+" && "+command)
		return err
	}}
}

// SnapshotStep creates a verified target backup and restores it on undo.
func SnapshotStep(r deploy.Remote, remotePath, tag string) Step {
	return Step{Name: "snapshot", Run: func(ctx context.Context, st *State) error {
		if err := deploy.ValidateRemotePath(remotePath); err != nil {
			return err
		}
		if !snapshotTagRE.MatchString(tag) {
			return fmt.Errorf("invalid promote snapshot tag %q", tag)
		}
		id, err := controlplane.VerifiedSnapshot(ctx, r, remotePath, tag)
		if err != nil {
			return err
		}
		st.SnapshotTag, st.BackupID, st.BackupHost = tag, id, r.Label()
		return nil
	}, Undo: func(ctx context.Context, st *State) error {
		if err := deploy.ValidateRemotePath(remotePath); err != nil {
			return err
		}
		if st.BackupID == "" {
			return fmt.Errorf("promote snapshot has no backup id")
		}
		_, err := r.Run(ctx, "cd "+remotesdk.ShellQuote(remotePath)+" && nself backup restore "+remotesdk.ShellQuote(st.BackupID)+" --yes")
		return err
	}}
}

// SaveStep reads the target manifests and stores them in State for the record.
func SaveStep(r deploy.Remote, remotePath string) Step {
	return Step{Name: "save", Run: func(ctx context.Context, st *State) error {
		if err := deploy.ValidateRemotePath(remotePath); err != nil {
			return err
		}
		read := func(rel string) ([]string, error) {
			file := path.Join(remotePath, rel)
			q := remotesdk.ShellQuote(file)
			out, err := r.Run(ctx, "if [ -f "+q+" ]; then cat "+q+"; fi")
			if err != nil {
				return nil, fmt.Errorf("reading %s on %s: %w", rel, r.Label(), err)
			}
			var lines []string
			for _, line := range strings.Split(string(out), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					lines = append(lines, line)
				}
			}
			return lines, nil
		}
		compose, err := read(".nself/compose-files.txt")
		if err != nil {
			return err
		}
		envs, err := read(".nself/compose-env-files.txt")
		if err != nil {
			return err
		}
		if st.Values == nil {
			st.Values = map[string]any{}
		}
		st.Values["saved_manifest"] = SavedManifest{Host: r.Label(), ComposeFiles: compose, EnvFiles: envs}
		return nil
	}}
}
