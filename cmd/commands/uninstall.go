package commands

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

// CLI-R19: `uninstall` is retired in favour of `reset`, whose flags now cover
// the same three modes. It stays REGISTERED (hidden) rather than being argv-
// rewritten onto `reset --purge`, because the two commands' defaults differ in
// how destructive they are: bare `nself uninstall` keeps database volumes,
// bare `nself reset` removes them. Rewriting one onto the other would turn a
// safe habit into data loss. Hidden-but-registered means the old invocation
// behaves EXACTLY as before, and the deprecation warning points at the
// equivalent reset flag.
var uninstallCmd = &cobra.Command{
	Use:    "uninstall",
	Hidden: true,
	Short:  "Deprecated: use `nself reset` (see --keep-data / --purge)",
	Long: `Remove nSelf-generated files and containers from the current project.

This command is interactive by default. Use --yes to skip confirmation prompts.

Flags:
  --keep-data   Remove containers and generated files, but keep the database
                volumes (Postgres data).
  --purge       Remove everything including database volumes (DESTRUCTIVE).
                Requires an extra confirmation prompt.
  --yes         Skip all confirmation prompts (combine with --purge carefully).

What is removed:
  Default (no flags):  docker-compose.yml, nginx/sites/, .nself/cache/
  --keep-data:         same as default + stops containers, keeps DB volumes
  --purge:             same as --keep-data + removes Postgres volumes

What is NEVER removed:
  .env*, .env.dev, .env.staging, .env.prod, .env.secrets
  nginx/conf.d/     (hand-managed nginx fragments)
  Any application source code you wrote

Examples:
  nself uninstall              # Interactive, keeps DB data
  nself uninstall --keep-data  # Same as default — explicit
  nself uninstall --purge      # Remove everything including DB (with confirmation)
  nself uninstall --purge --yes  # Non-interactive full purge`,
	RunE: runUninstall,
}

func init() {
	uninstallCmd.Flags().Bool("keep-data", false, "Keep database volumes (default behaviour)")
	uninstallCmd.Flags().Bool("purge", false, "Remove everything including database volumes (DESTRUCTIVE)")
	uninstallCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	RootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	res, done, err := uninstallProject(cmd)
	if err != nil || !done {
		return err
	}
	return emitBuildResult(cmd, "uninstall", res)
}

// uninstallProject is the original uninstall implementation, shared with
// `reset --keep-data|--purge`. done is false when the user declined a prompt
// (nothing was removed); in JSON mode that is an error, not a silent exit 0.
func uninstallProject(cmd *cobra.Command) (res UninstallResult, done bool, err error) {
	keepData, _ := cmd.Flags().GetBool("keep-data")
	purge, _ := cmd.Flags().GetBool("purge")
	yes, _ := cmd.Flags().GetBool("yes")

	// --keep-data and --purge are mutually exclusive.
	if keepData && purge {
		return res, false, fmt.Errorf("--keep-data and --purge are mutually exclusive — choose one")
	}

	// Locate project root.
	cwd, err := os.Getwd()
	if err != nil {
		return res, false, fmt.Errorf("getting working directory: %w", err)
	}

	// Verify this looks like an nSelf project.
	nSelfDir := filepath.Join(cwd, ".nself")
	if _, statErr := os.Stat(nSelfDir); os.IsNotExist(statErr) {
		envPath := filepath.Join(cwd, ".env.dev")
		if _, envErr := os.Stat(envPath); os.IsNotExist(envErr) {
			return res, false, fmt.Errorf("no nSelf project found in %s — nothing to uninstall", cwd)
		}
	}

	ui.CommandHeader("nself uninstall", "Remove nSelf-generated files and containers")

	// Describe what will happen.
	fmt.Println()
	if purge {
		fmt.Printf("  %s This will stop containers and delete ALL generated files\n",
			ui.C(ui.Red, ui.IconWarning))
		fmt.Printf("  %s Database volumes will be PERMANENTLY deleted.\n",
			ui.C(ui.Red, ui.IconWarning))
	} else {
		fmt.Printf("  %s This will stop containers and remove generated files.\n",
			ui.C(ui.Blue, ui.IconInfo))
		fmt.Printf("  %s Database volumes will be preserved.\n",
			ui.C(ui.Green, ui.IconSuccess))
	}
	fmt.Printf("  %s Your .env* files and source code are not touched.\n",
		ui.C(ui.Green, ui.IconSuccess))
	fmt.Println()

	// Confirmation.
	if !yes {
		if !confirmPrompt(cmd, "Continue with uninstall? [y/N] ") {
			fmt.Println("Aborted.")
			return res, false, abortedUninstall(cmd)
		}
	}

	// Extra confirmation for --purge.
	if purge && !yes {
		fmt.Println()
		fmt.Printf("  %s DANGER: This will permanently delete your database.\n",
			ui.C(ui.Red, ui.IconWarning))
		if !confirmPrompt(cmd, "Type 'purge' to confirm: ") {
			fmt.Println("Aborted.")
			return res, false, abortedUninstall(cmd)
		}
	}

	steps := ui.NewInitSteps(false,
		"Stopping containers",
		"Removing generated compose file",
		"Removing nginx site configs",
		"Clearing .nself/cache",
	)
	if purge {
		steps = ui.NewInitSteps(false,
			"Stopping containers",
			"Removing generated compose file",
			"Removing nginx site configs",
			"Clearing .nself/cache",
			"Removing database volumes",
		)
	}

	steps.Next() // Stopping containers
	if err := runDockerComposeDown(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), cwd, purge); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s docker compose down: %v (continuing)\n",
			ui.C(ui.Yellow, ui.IconWarning), err)
	}

	removed := []string{}
	steps.Next() // Remove compose file
	removed = recordIfExists(removed, cwd, "docker-compose.yml", removeIfExists)

	steps.Next() // Remove nginx sites
	sites, _ := filepath.Glob(filepath.Join(cwd, "nginx", "sites", "*.conf"))
	for _, m := range sites {
		rel, relErr := filepath.Rel(cwd, m)
		if relErr != nil {
			rel = m
		}
		removed = recordIfExists(removed, cwd, filepath.ToSlash(rel), removeIfExists)
	}

	steps.Next() // Clear .nself/cache
	removed = recordIfExists(removed, cwd, ".nself/cache/", removeDir)

	if purge {
		steps.Next() // Remove volumes
		removed = recordIfExists(removed, cwd, ".nself/volumes/", removeDir)
	}

	steps.Done()

	fmt.Println()
	fmt.Printf("  %s nSelf project files removed.\n", ui.C(ui.Green, ui.IconSuccess))
	fmt.Println()
	fmt.Printf("  To start fresh: %s\n", ui.C(ui.Cyan, "nself init --force && nself build && nself start"))
	fmt.Println()
	return UninstallResult{Removed: removed, KeptData: !purge}, true, nil
}

// abortedUninstall is the result of a declined prompt: nil in human mode (the
// command has always exited 0 there), a destructive_blocked error in JSON mode
// so a machine caller does not read a refusal as success.
func abortedUninstall(cmd *cobra.Command) error {
	if buildEnvelopeWanted(cmd) {
		return errConfirmRequired("uninstall")
	}
	return nil
}

// recordIfExists runs remove on project-relative rel when it exists and
// appends rel to removed. Trailing "/" in rel marks a directory in the report.
func recordIfExists(removed []string, root, rel string, remove func(string)) []string {
	path := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(rel, "/")))
	if _, err := os.Lstat(path); err != nil {
		return removed
	}
	remove(path)
	return append(removed, rel)
}

// runDockerComposeDown stops and removes containers for the project.
// If purge is true it also removes volumes.
// ctx is used to cancel the docker subprocess if the command context fires
// (e.g. test harness deadline), preventing orphan I/O after the test exits.
func runDockerComposeDown(ctx context.Context, stdout, stderr io.Writer, projectDir string, purge bool) error {
	composeFile := filepath.Join(projectDir, "docker-compose.yml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return nil // nothing to stop
	}

	dockerArgs := []string{"compose", "--file", composeFile, "down"}
	if purge {
		dockerArgs = append(dockerArgs, "--volumes")
	}
	dcmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	dcmd.Stdout = stdout
	dcmd.Stderr = stderr
	dcmd.WaitDelay = 5 * time.Second
	return dcmd.Run()
}

// confirmPrompt prints the given prompt and returns true if the user typed
// "y", "yes", or "purge" (for the purge confirmation).
//
// cmd is the cobra.Command whose InOrStdin() is used as the input source.
// This allows tests to inject a closed reader (EOF) via root.SetIn() rather
// than blocking forever on the real os.Stdin.
func confirmPrompt(cmd *cobra.Command, prompt string) bool {
	_, _ = fmt.Fprint(cmd.OutOrStdout(), prompt)
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "y" || answer == "yes" || answer == "purge"
}

// removeIfExists deletes a single file if it exists; ignores missing files.
func removeIfExists(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "  %s removing %s: %v\n",
			ui.C(ui.Yellow, ui.IconWarning), path, err)
	}
}

// removeDir removes a directory and its contents; ignores missing directories.
func removeDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		fmt.Fprintf(os.Stderr, "  %s removing directory %s: %v\n",
			ui.C(ui.Yellow, ui.IconWarning), path, err)
	}
}
