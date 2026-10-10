package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/confirm"

	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove generated artifacts (docker-compose.yml, nginx configs, build cache)",
	Long: `Clean removes generated artifacts that are recreated by 'nself build'.

Removed:
  - docker-compose.yml
  - nginx/sites/ configuration files
  - .nself/cache/ (build cache)
  - Docker build cache (exec.cachemount type)

Preserved:
  - .env and all .env.* variants
  - Docker volumes and container data
  - User-managed files

No confirmation required — this operation is non-destructive (data is preserved).
Run 'nself build' after clean to regenerate all artifacts.

With --all, clean additionally runs a host-wide 'docker system prune',
removing unused containers, networks, images, and build cache for EVERY
Docker project on the machine, not just this one. Named volumes are never
touched. --all requires typing "yes" at an interactive prompt; pass --yes
to skip the prompt for scripted or CI use.`,
	RunE: runClean,
}

func init() {
	cleanCmd.Flags().Bool("all", false, "Also run a host-wide 'docker system prune' affecting every Docker project on this machine (destructive; requires confirmation)")
	cleanCmd.Flags().Bool("yes", false, "Skip the --all confirmation prompt (for CI/CD)")
	RootCmd.AddCommand(cleanCmd)
}

// dockerSystemPrune runs a host-wide `docker system prune`, removing unused
// containers, networks, images, and build cache for every Docker project on
// the machine. It deliberately never passes --volumes: named volumes hold
// other projects' database and storage data, and deleting them requires its
// own explicit opt-in and confirmation, which this command does not yet
// offer. It is a package variable so tests can swap in a spy and assert
// whether it was invoked without needing a Docker daemon.
var dockerSystemPrune = func(ctx context.Context, out, errOut io.Writer) error {
	pruneCmd := exec.CommandContext(ctx, "docker", "system", "prune", "--all", "--force")
	pruneCmd.Stdout = out
	pruneCmd.Stderr = errOut
	return pruneCmd.Run()
}

func runClean(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Find project root (best effort — clean can still work without one)
	projectDir := cwd
	if root, findErr := config.FindNSelfRoot(cwd); findErr == nil {
		projectDir = root
	}

	var removed []string

	// 1. docker-compose.yml
	composePath := filepath.Join(projectDir, "docker-compose.yml")
	if _, statErr := os.Stat(composePath); statErr == nil {
		if removeErr := os.Remove(composePath); removeErr == nil {
			removed = append(removed, "docker-compose.yml")
		} else {
			fmt.Fprintf(os.Stderr, "Warning: could not remove docker-compose.yml: %v\n", removeErr)
		}
	}

	// 2. nginx/sites/ configuration files
	nginxSitesDir := filepath.Join(projectDir, "nginx", "sites")
	if info, statErr := os.Stat(nginxSitesDir); statErr == nil && info.IsDir() {
		entries, readErr := os.ReadDir(nginxSitesDir)
		if readErr == nil {
			for _, entry := range entries {
				name := entry.Name()
				if strings.HasSuffix(name, ".conf") {
					entryPath := filepath.Join(nginxSitesDir, name)
					if removeErr := os.Remove(entryPath); removeErr == nil {
						removed = append(removed, filepath.Join("nginx", "sites", name))
					}
				}
			}
		}
	}

	// 3. .nself/cache/ directory
	nSelfCacheDir := filepath.Join(projectDir, ".nself", "cache")
	if _, statErr := os.Stat(nSelfCacheDir); statErr == nil {
		if removeErr := os.RemoveAll(nSelfCacheDir); removeErr == nil {
			removed = append(removed, ".nself/cache/")
		} else {
			fmt.Fprintf(os.Stderr, "Warning: could not remove .nself/cache/: %v\n", removeErr)
		}
	}

	// 4. Docker build cache (non-fatal if docker not available)
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Pruning Docker build cache...")
	pruneCmd := exec.CommandContext(cmd.Context(), "docker", "builder", "prune",
		"--filter", "type=exec.cachemount",
		"--force",
	)
	pruneOut, pruneErr := pruneCmd.CombinedOutput()
	if pruneErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: docker builder prune failed (Docker may not be running): %v\n", pruneErr)
	} else {
		trimmed := strings.TrimSpace(string(pruneOut))
		if trimmed != "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), trimmed)
		}
	}

	// Summary
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out)
	if len(removed) == 0 {
		_, _ = fmt.Fprintln(out, "Nothing to remove — already clean.")
	} else {
		_, _ = fmt.Fprintf(out, "Removed %d file(s):\n", len(removed))
		for _, r := range removed {
			_, _ = fmt.Fprintf(out, "  - %s\n", r)
		}
		_, _ = fmt.Fprintln(out, "\nRun 'nself build' to regenerate.")
	}

	// 5. --all: host-wide docker system prune, gated behind confirmation.
	all, _ := cmd.Flags().GetBool("all")
	if !all {
		return emitBuildResult(cmd, "clean", CleanResult{Removed: nonNil(removed)})
	}

	skipConfirm, _ := cmd.Flags().GetBool("yes")
	if !skipConfirm {
		if confirmErr := confirm.ConfirmHostWidePrune(cmd.InOrStdin(), out); confirmErr != nil {
			_, _ = fmt.Fprintln(out, "\nHost-wide prune canceled.")
			if buildEnvelopeWanted(cmd) {
				return errConfirmRequired("clean --all")
			}
			return nil
		}
	}

	_, _ = fmt.Fprintln(out, "\nPruning all unused Docker resources on this host...")
	pruned := false
	if pruneAllErr := dockerSystemPrune(cmd.Context(), out, cmd.ErrOrStderr()); pruneAllErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: docker system prune failed (Docker may not be running): %v\n", pruneAllErr)
	} else {
		_, _ = fmt.Fprintln(out, "Host-wide prune complete.")
		pruned = true
	}

	return emitBuildResult(cmd, "clean", CleanResult{Removed: nonNil(removed), HostPrune: pruned})
}
