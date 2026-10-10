package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/confirm"

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
		if err := confirm.ConfirmDestruction(projectName, os.Stdin, os.Stdout); err != nil {
			fmt.Println("Destruction canceled.")
			if buildEnvelopeWanted(cmd) {
				return errConfirmRequired("reset")
			}
			return nil
		}
	}

	// Stop containers and remove volumes
	composePath := filepath.Join(projectDir, "docker-compose.yml")
	if _, statErr := os.Stat(composePath); statErr == nil {
		fmt.Println("Stopping containers and removing volumes...")
		dcCmd := exec.CommandContext(cmd.Context(), "docker", "compose", "down", "--volumes", "--remove-orphans")
		dcCmd.Dir = projectDir
		dcCmd.Stdout = os.Stdout
		dcCmd.Stderr = os.Stderr
		if err := dcCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: docker compose down reported an error: %v\n", err)
		} else {
			fmt.Println("  Containers stopped and volumes removed.")
		}
	} else {
		fmt.Println("No docker-compose.yml found — skipping container shutdown.")
	}

	// Remove generated files
	var removed []string

	removed, _ = removeFile(filepath.Join(projectDir, "docker-compose.yml"), removed, []string{})
	removed, _ = removeFile(filepath.Join(projectDir, ".env.computed"), removed, []string{})

	// nginx/sites/ contents
	nginxSitesDir := filepath.Join(projectDir, "nginx", "sites")
	if info, statErr := os.Stat(nginxSitesDir); statErr == nil && info.IsDir() {
		entries, _ := os.ReadDir(nginxSitesDir)
		for _, entry := range entries {
			entryPath := filepath.Join(nginxSitesDir, entry.Name())
			if removeErr := os.RemoveAll(entryPath); removeErr == nil {
				removed = append(removed, filepath.Join("nginx", "sites", entry.Name()))
			}
		}
	}

	// ssl/ contents
	sslDir := filepath.Join(projectDir, "ssl")
	if info, statErr := os.Stat(sslDir); statErr == nil && info.IsDir() {
		entries, _ := os.ReadDir(sslDir)
		for _, entry := range entries {
			entryPath := filepath.Join(sslDir, entry.Name())
			if removeErr := os.RemoveAll(entryPath); removeErr == nil {
				removed = append(removed, filepath.Join("ssl", entry.Name()))
			}
		}
	}

	fmt.Println()
	if len(removed) == 0 {
		fmt.Println("No generated files found — project is already clean.")
	} else {
		fmt.Printf("Removed %d generated file(s).\n", len(removed))
	}
	fmt.Println("Reset complete. Run 'nself init' or 'nself build' to recreate the stack.")
	return emitBuildResult(cmd, "reset", ResetResult{Removed: nonNil(removed), KeptData: false})
}

// removeFile removes a single file by path, appending its relative name (relative
// to its containing directory for display) to the removed or notFound slice.
func removeFile(path string, removed, notFound []string) ([]string, []string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		notFound = append(notFound, filepath.Base(path))
		return removed, notFound
	}
	if err := os.Remove(path); err != nil {
		// Append to notFound with an error note rather than propagating so the
		// caller sees a full summary. Callers that care about hard errors should
		// call os.Remove directly.
		notFound = append(notFound, fmt.Sprintf("%s (remove failed: %v)", filepath.Base(path), err))
		return removed, notFound
	}
	removed = append(removed, filepath.Base(path))
	return removed, notFound
}
