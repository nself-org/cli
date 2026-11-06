// Package restoreconsent holds the operator-consent rule for replacing a project
// database with a backup, and the one `nself backup restore` command line that
// deploy rollback and promote rollback run over ssh (C2-01).
//
// Inputs: the project name and production flag, the --yes and
// --confirm-project values, a prompt reader and writer; for the remote form a runner, path and backup id.
// Outputs: nil when consent is given, else an error; the command line or the
// hint an operator runs by hand.
// Constraints: bare --yes never authorizes a production restore. An ssh run
// has no stdin, so remote callers confirm with the operator first and then
// pass --confirm-project, which the host checks against its own PROJECT_NAME.
// --yes stays on the remote command so an older CLI fails on the unknown flag
// instead of prompting. Stdlib and the sdk shell quoting only, so deploy and
// promote can import it from below (layering, config fan-in ratchets).
package restoreconsent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Confirm reads consent before a restore replaces the database: a typed "yes"
// (or yes) off production, the typed project name on production, or proof
// equal to the project name anywhere, which reads no prompt. A wrong proof is
// refused everywhere.
func Confirm(in io.Reader, out io.Writer, project string, production, yes bool, proof string) error {
	if proof != "" {
		if project == "" || proof != project {
			return fmt.Errorf("restore confirmation failed: --confirm-project %q does not match project %q", proof, project)
		}
		return nil
	}
	if !production && yes {
		return nil
	}
	expected, warning := "yes", "WARNING"
	if production {
		expected, warning = project, "WARNING: PRODUCTION"
		if expected == "" {
			return fmt.Errorf("restore confirmation requires a non-empty production project name")
		}
	}
	if _, err := fmt.Fprintf(out, "%s: This will replace the database for project %q.\n", warning, project); err != nil {
		return fmt.Errorf("write restore confirmation warning: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Type %q to confirm: ", expected); err != nil {
		return fmt.Errorf("write restore confirmation prompt: %w", err)
	}
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != expected {
		return fmt.Errorf("restore confirmation failed: expected %q", expected)
	}
	return nil
}

// Command returns `nself backup restore <id> --yes --confirm-project <project>`
// with both values shell-quoted; an empty project is an error.
func Command(backupID, project string) (string, error) {
	if strings.TrimSpace(project) == "" {
		return "", fmt.Errorf("restoring backup %s needs the project name for --confirm-project; nothing was restored", backupID)
	}
	return fmt.Sprintf("nself backup restore %s --yes --confirm-project %s", remote.ShellQuote(backupID), remote.ShellQuote(project)), nil
}

// Precheck refuses a pending database restore that has no project name, so a
// rollback stops before it changes any host. restored: already done.
func Precheck(backupID, project string, restored bool) error {
	if backupID == "" || restored {
		return nil
	}
	_, err := Command(backupID, project)
	return err
}

// Hint is the restore command an operator runs by hand on the backup host; an
// unknown project prints the <project> placeholder.
func Hint(backupID, project string) string {
	if p := strings.TrimSpace(project); p != "" {
		project = remote.ShellQuote(p)
	} else {
		project = "<project>"
	}
	return fmt.Sprintf("nself backup restore %s --confirm-project %s", remote.ShellQuote(backupID), project)
}

// Runner is the part of a deploy remote RunRestore needs.
type Runner interface {
	Run(ctx context.Context, cmd string) ([]byte, error)
	Label() string
}

// RunRestore runs the restore on r in remotePath through sh -c, then onDone
// (may be nil) so the caller records the restore before anything else runs.
func RunRestore(ctx context.Context, r Runner, remotePath, backupID, project string, out io.Writer, onDone func() error) error {
	restoreCmd, err := Command(backupID, project)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "  [running] restore database backup %s\n", backupID)
	script := fmt.Sprintf("cd %s && %s", remote.ShellQuote(remotePath), restoreCmd)
	if _, err := r.Run(ctx, "sh -c "+remote.ShellQuote(script)); err != nil {
		return fmt.Errorf("restoring backup %s on %s: %w; the files are restored, the stack was not restarted", backupID, r.Label(), err)
	}
	if onDone != nil {
		if err := onDone(); err != nil {
			return fmt.Errorf("the database backup was restored but the record could not be saved (do NOT run the rollback again): %w", err)
		}
	}
	return nil
}
