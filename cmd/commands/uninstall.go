package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

	// Extra confirmation for --purge: only the word itself passes, so a
	// stream of "y" answers cannot reach the database.
	if purge && !yes {
		fmt.Println()
		fmt.Printf("  %s DANGER: This will permanently delete your database.\n",
			ui.C(ui.Red, ui.IconWarning))
		if !confirmWord(cmd, "Type 'purge' to confirm: ", "purge") {
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

	jsonMode := buildEnvelopeWanted(cmd)

	steps.Next() // Stopping containers
	// volumesDeleted is true only when compose down --volumes really ran: with no
	// compose file nothing touched the named volumes, so the data is kept.
	ran, err := composeDown(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), cwd, purge)
	volumesDeleted := purge && ran && err == nil
	if err != nil {
		// JSON mode stops here, before the compose file is deleted: the
		// containers (and under --purge the volumes) are still there, and the
		// caller needs the file to retry the shutdown.
		if jsonMode {
			return res, false, errPartialRemoval("docker compose down failed, so containers and volumes may still exist", err, nil)
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s docker compose down: %v (continuing)\n",
			ui.C(ui.Yellow, ui.IconWarning), err)
	}

	removed := []string{}
	var failed []error
	record := func(rel string, remove func(string) error) {
		var err error
		removed, err = recordIfExists(removed, cwd, rel, remove)
		if err != nil {
			failed = append(failed, err)
		}
	}
	steps.Next() // Remove compose file
	record("docker-compose.yml", removeIfExists)

	steps.Next() // Remove nginx sites
	sites, _ := filepath.Glob(filepath.Join(cwd, "nginx", "sites", "*.conf"))
	for _, m := range sites {
		rel, relErr := filepath.Rel(cwd, m)
		if relErr != nil {
			rel = m
		}
		record(filepath.ToSlash(rel), removeIfExists)
	}

	steps.Next() // Clear .nself/cache
	record(".nself/cache/", removeDir)

	if purge {
		steps.Next() // Remove volumes
		record(".nself/volumes/", removeDir)
	}

	steps.Done()

	// A path that could not be deleted is never reported as removed, and in
	// JSON mode any such failure fails the command: kept_data false after a
	// failed purge would tell the caller the data is gone when it is not.
	if len(failed) > 0 && jsonMode {
		done := removed
		if volumesDeleted {
			done = append([]string{volumesGone}, removed...)
		}
		return res, false, errRemovalFailed("could not remove every path", errors.Join(failed...), done)
	}

	fmt.Println()
	if len(failed) > 0 {
		// Human mode: the warnings above named each path; no success line.
		// Exit status stays 0, as for removal failures.
		printIncompleteRemoval(len(failed))
		return UninstallResult{Removed: removed, KeptData: uninstallKeptData(volumesDeleted, removed)}, true, nil
	}
	fmt.Printf("  %s nSelf project files removed.\n", ui.C(ui.Green, ui.IconSuccess))
	fmt.Println()
	fmt.Printf("  To start fresh: %s\n", ui.C(ui.Cyan, "nself init --force && nself build && nself start"))
	fmt.Println()
	return UninstallResult{Removed: removed, KeptData: uninstallKeptData(volumesDeleted, removed)}, true, nil
}

// uninstallKeptData reports whether database data was left in place. It is
// gone when compose down --volumes ran, and also when the .nself/volumes/
// directory (bind-mounted data) was removed (D-0375).
func uninstallKeptData(volumesDeleted bool, removed []string) bool {
	return !volumesDeleted && !slices.Contains(removed, ".nself/volumes/")
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

// composeDown stops the project's containers and, when purge is true, removes
// its volumes. A package variable so tests can make it fail without Docker.
var composeDown = runDockerComposeDown

// runDockerComposeDown stops and removes containers for the project.
// If purge is true it also removes volumes. ran is false when there is no
// compose file, so nothing was stopped and no volume was touched.
// ctx is used to cancel the docker subprocess if the command context fires
// (e.g. test harness deadline), preventing orphan I/O after the test exits.
func runDockerComposeDown(ctx context.Context, stdout, stderr io.Writer, projectDir string, purge bool) (ran bool, err error) {
	composeFile := filepath.Join(projectDir, "docker-compose.yml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return false, nil // nothing to stop
	}

	dockerArgs := []string{"compose", "--file", composeFile, "down"}
	if purge {
		dockerArgs = append(dockerArgs, "--volumes")
	}
	dcmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	dcmd.Stdout = stdout
	dcmd.Stderr = stderr
	dcmd.WaitDelay = 5 * time.Second
	return true, dcmd.Run()
}

// confirmPrompt prints the given prompt and returns true if the user typed
// "y", "yes", or "purge" (for the purge confirmation).
//
// cmd is the cobra.Command whose InOrStdin() is used as the input source.
// This allows tests to inject a closed reader (EOF) via root.SetIn() rather
// than blocking forever on the real os.Stdin.
func confirmPrompt(cmd *cobra.Command, prompt string) bool {
	_, _ = fmt.Fprint(cmd.OutOrStdout(), prompt)
	answer, ok := readAnswer(cmd.InOrStdin())
	return ok && (answer == "y" || answer == "yes" || answer == "purge")
}

// confirmWord prints prompt and returns true only when the user typed word
// (case-insensitive). It is the gate for prompts that name what to type.
func confirmWord(cmd *cobra.Command, prompt, word string) bool {
	_, _ = fmt.Fprint(cmd.OutOrStdout(), prompt)
	answer, ok := readAnswer(cmd.InOrStdin())
	return ok && answer == strings.ToLower(word)
}
