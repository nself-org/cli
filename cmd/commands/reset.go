package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/confirm"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Stop containers, remove all data volumes, and clean generated files",
	Long: `Reset stops all containers, removes Docker volumes (all project data including
database, storage, Redis), and removes generated files (docker-compose.yml,
nginx configs). Your .env file is preserved.

⚠️  This is destructive and irreversible. All project data will be deleted.

Use --yes to skip the interactive confirmation (for CI/CD pipelines).`,
	RunE: runReset,
}

func init() {
	resetCmd.Flags().Bool("yes", false, "Skip confirmation prompt (for CI/CD)")
	resetCmd.Flags().Bool("no-monorepo", false, "Disable automatic monorepo backend detection")
	// CLI-R19: reset absorbs the retired `uninstall` command's two non-default
	// modes so the replacement named in the deprecation warning actually exists.
	// Both delegate to runUninstall, which is the original implementation — a
	// second copy would drift on exactly the paths that delete data.
	resetCmd.Flags().Bool("keep-data", false, "Remove containers and generated files but keep database volumes")
	resetCmd.Flags().Bool("purge", false, "Remove everything including database volumes (DESTRUCTIVE)")
	RootCmd.AddCommand(resetCmd)
}

func runReset(cmd *cobra.Command, args []string) error {
	// --keep-data and --purge select the behaviour the retired `uninstall`
	// command had under those flags. Bare `nself reset` keeps its own, older
	// meaning (remove volumes and generated files, preserve .env), which is
	// why this dispatch exists rather than a blanket redirect.
	keepData, _ := cmd.Flags().GetBool("keep-data")
	purge, _ := cmd.Flags().GetBool("purge")
	if keepData || purge {
		res, done, err := uninstallProject(cmd)
		if err != nil || !done {
			return err
		}
		return emitBuildResult(cmd, "reset", ResetResult(res))
	}

	skipConfirm, _ := cmd.Flags().GetBool("yes")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	noMonorepo, _ := cmd.Flags().GetBool("no-monorepo")
	if !noMonorepo {
		if backendRoot := config.DetectMonorepoRoot(cwd); backendRoot != "" {
			fmt.Printf("→ Detected monorepo layout. Using %s as project root.\n", filepath.Base(backendRoot))
			cwd = backendRoot
		}
	}

	projectDir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return fmt.Errorf("no nself project found in current directory or parents. Run 'nself init' to create a project")
	}

	projectName := filepath.Base(projectDir)

	// Load config to get base domain (best effort — don't fail if config missing)
	var baseDomain string
	if cfg, cfgErr := config.Load(projectDir); cfgErr == nil {
		baseDomain = cfg.BaseDomain
	}

	// Safety confirmation
	if !skipConfirm {
		if baseDomain != "" {
			fmt.Printf("   Project: %s (%s)\n", projectName, baseDomain)
		}
		if err := confirm.ConfirmDestruction(projectName, cmd.InOrStdin(), cmd.OutOrStdout()); err != nil {
			fmt.Println("Destruction canceled.")
			if buildEnvelopeWanted(cmd) {
				return errConfirmRequired("reset")
			}
			return nil
		}
	}

	// Stop containers and remove volumes. volumesRemoved drives kept_data:
	// the envelope only says the data is gone when compose down --volumes
	// actually succeeded.
	volumesRemoved := false
	composePath := filepath.Join(projectDir, "docker-compose.yml")
	if _, statErr := os.Stat(composePath); statErr == nil {
		fmt.Println("Stopping containers and removing volumes...")
		if err := resetComposeDown(cmd.Context(), projectDir); err != nil {
			// JSON mode stops before deleting the compose file the caller
			// needs to retry; human mode warns and carries on as before.
			if buildEnvelopeWanted(cmd) {
				return errPartialRemoval("docker compose down --volumes failed, so containers and volumes may still exist", err, nil)
			}
			fmt.Fprintf(os.Stderr, "Warning: docker compose down reported an error: %v\n", err)
		} else {
			fmt.Println("  Containers stopped and volumes removed.")
			volumesRemoved = true
		}
	} else {
		fmt.Println("No docker-compose.yml found — skipping container shutdown.")
	}

	// Remove generated files. A path that could not be deleted is never
	// reported as removed; in JSON mode any such failure fails the command.
	var removed []string
	var failed []error
	record := func(rel string, remove func(string) error) {
		var err error
		if removed, err = recordIfExists(removed, projectDir, rel, remove); err != nil {
			failed = append(failed, err)
		}
	}
	record("docker-compose.yml", removeIfExists)
	record(".env.computed", removeIfExists)
	// nginx/sites/ and ssl/ contents
	for _, dir := range []struct{ abs, rel string }{
		{filepath.Join(projectDir, "nginx", "sites"), path.Join("nginx", "sites")},
		{filepath.Join(projectDir, "ssl"), "ssl"},
	} {
		// A directory that is absent is nothing to remove; one that cannot be
		// inspected or listed is a failure, not "already clean" (D-0375).
		info, statErr := os.Stat(dir.abs)
		if statErr != nil {
			if !errors.Is(statErr, os.ErrNotExist) {
				failed = append(failed, fmt.Errorf("checking %s: %w", dir.rel, statErr))
			}
			continue
		}
		if !info.IsDir() {
			continue
		}
		entries, readErr := os.ReadDir(dir.abs)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			failed = append(failed, fmt.Errorf("listing %s: %w", dir.rel, readErr))
		}
		for _, entry := range entries {
			record(dir.rel+"/"+entry.Name(), removeDir)
		}
	}
	if len(failed) > 0 && buildEnvelopeWanted(cmd) {
		done := removed
		if volumesRemoved {
			done = append([]string{volumesGone}, removed...)
		}
		return errRemovalFailed("could not remove every generated file", errors.Join(failed...), done)
	}

	fmt.Println()
	if len(removed) == 0 {
		fmt.Println("No generated files found — project is already clean.")
	} else {
		fmt.Printf("Removed %d generated file(s).\n", len(removed))
	}
	fmt.Println("Reset complete. Run 'nself init' or 'nself build' to recreate the stack.")
	return emitBuildResult(cmd, "reset", ResetResult{Removed: jsonPaths(removed), KeptData: !volumesRemoved})
}

// resetComposeDown runs `docker compose down --volumes --remove-orphans` in
// projectDir. A package variable so tests can make it fail without Docker.
var resetComposeDown = func(ctx context.Context, projectDir string) error {
	dcCmd := exec.CommandContext(ctx, "docker", "compose", "down", "--volumes", "--remove-orphans")
	dcCmd.Dir = projectDir
	dcCmd.Stdout = os.Stdout
	dcCmd.Stderr = os.Stderr
	return dcCmd.Run()
}

// recordIfExists runs remove on project-relative rel when it exists and
// appends rel to removed only when the removal succeeded; the removal error is
// returned otherwise. A path that is absent (ENOENT) is skipped silently; an
// Lstat failure of any other kind is returned as an error. Trailing "/" in rel marks a directory in the report.
func recordIfExists(removed []string, root, rel string, remove func(string) error) ([]string, error) {
	path := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(rel, "/")))
	if _, err := os.Lstat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return removed, nil // absent: nothing to remove
		}
		// Any other error (EACCES on a parent, ELOOP, ...) means the path
		// could not be inspected, not that it is gone: report it so the caller
		// counts it as a failure instead of "already clean".
		return removed, fmt.Errorf("checking %s: %w", rel, err)
	}
	if err := remove(path); err != nil {
		return removed, fmt.Errorf("removing %s: %w", rel, err)
	}
	return append(removed, rel), nil
}

// removeIfExists deletes a single file; a missing file is not an error. A
// failure is warned about on stderr and returned.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "  %s removing %s: %v\n",
			ui.C(ui.Yellow, ui.IconWarning), path, err)
		return err
	}
	return nil
}

// removeDir removes a directory and its contents; a missing directory is not
// an error. A failure is warned about on stderr and returned.
func removeDir(path string) error {
	if err := removeAll(path); err != nil {
		fmt.Fprintf(os.Stderr, "  %s removing directory %s: %v\n",
			ui.C(ui.Yellow, ui.IconWarning), path, err)
		return err
	}
	return nil
}

// removeAll is os.RemoveAll, a package variable so tests can make a removal
// fail the way EACCES does without depending on file-permission semantics.
var removeAll = os.RemoveAll

// readAnswer reads one line from r a byte at a time and returns it trimmed and
// lower-cased; ok is false at EOF with nothing read. It never reads past the
// newline, so consecutive prompts on one piped stream each get their own line
// (a bufio.Scanner per prompt would swallow the later answers).
// A line longer than maxAnswerLen, or a reader that keeps returning nothing,
// is a refusal, so `</dev/zero` cannot grow memory or spin forever.
func readAnswer(r io.Reader) (answer string, ok bool) {
	const maxAnswerLen, maxEmptyReads = 1024, 100
	var line []byte
	buf := make([]byte, 1)
	for empty := 0; ; {
		n, err := r.Read(buf)
		switch {
		case n == 1 && buf[0] == '\n':
			return strings.TrimSpace(strings.ToLower(string(line))), true
		case n == 1:
			empty = 0
			if line = append(line, buf[0]); len(line) > maxAnswerLen {
				return "", false
			}
		case err == nil:
			if empty++; empty >= maxEmptyReads {
				return "", false
			}
		}
		if err != nil {
			if len(line) == 0 {
				return "", false
			}
			return strings.TrimSpace(strings.ToLower(string(line))), true
		}
	}
}
