package commands

// Purpose: RunE implementations for "nself db hasura ..." subcommands (console,
// metadata apply/export/reload, checksum verify/reset, diff, validate). Inputs
// are the cobra command/args; outputs are printed results or an error.
// Constraints: split out of db.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/nself-org/cli/internal/database"

	"github.com/spf13/cobra"
)

func runDBHasuraConsole(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	port := cfg.Hasura.Port
	if port == 0 {
		port = 8080
	}
	url := fmt.Sprintf("http://localhost:%d/console", port)
	setData(cmd, dbConsoleData{URL: url})
	fmt.Printf("Opening Hasura Console: %s\n", url)

	// Attempt to open the browser. Best-effort on macOS/Linux.
	_ = exec.Command("open", url).Start()

	return nil
}

func runDBHasuraMetadataApply(cmd *cobra.Command, _ []string) error {
	if handled, err := dispatchRemoteIfNeeded(cmd, "db", "hasura", "metadata", "apply"); handled {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	if err := database.HasuraApplyMetadata(cmd.Context(), cfg, dir); err != nil {
		return fmt.Errorf("hasura metadata apply: %w", err)
	}
	fmt.Println("Hasura metadata applied.")
	return nil
}

func runDBHasuraMetadataExport(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	outDir, err := database.HasuraExportToYAML(cmd.Context(), cfg, dir)
	if err != nil {
		// Fall back to raw JSON dump.
		data, jsonErr := database.HasuraExportMetadata(cmd.Context(), cfg)
		if jsonErr != nil {
			return fmt.Errorf("hasura metadata export: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("Metadata exported to %s\n", outDir)
	return nil
}

func runDBHasuraMetadataReload(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	if err := database.HasuraReloadMetadata(cmd.Context(), cfg); err != nil {
		return fmt.Errorf("hasura metadata reload: %w", err)
	}
	fmt.Println("Hasura metadata reloaded.")
	return nil
}

func runDBVerifyChecksums(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	plugin, _ := cmd.Flags().GetString("plugin")
	mismatches, err := database.VerifyChecksums(cmd.Context(), cfg, plugin)
	if err != nil {
		return fmt.Errorf("verify checksums: %w", err)
	}
	if len(mismatches) == 0 {
		fmt.Println("All checksums verified.")
		return nil
	}
	for _, m := range mismatches {
		fmt.Printf("MISMATCH %s (%s): stored=%s disk=%s\n", m.ID, m.Name, m.Expected[:12]+"...", m.Actual[:12]+"...")
	}
	return fmt.Errorf("%d checksum mismatch(es) found", len(mismatches))
}

func runDBResetChecksum(cmd *cobra.Command, args []string) error {
	safety, _ := cmd.Flags().GetBool("i-know-what-im-doing")
	if !safety {
		return fmt.Errorf("this command modifies migration tracking; pass --i-know-what-im-doing to confirm")
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	if err := database.ResetChecksum(cmd.Context(), cfg, args[0]); err != nil {
		return fmt.Errorf("reset checksum: %w", err)
	}
	fmt.Printf("Checksum reset for migration %s.\n", args[0])
	return nil
}

func runDBHasuraDiff(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	diffs, err := database.HasuraDiffMetadata(cmd.Context(), cfg, dir)
	if err != nil {
		return fmt.Errorf("hasura diff: %w", err)
	}

	if len(diffs) == 0 {
		fmt.Println("No metadata drift detected.")
		return nil
	}

	fmt.Printf("Metadata drift detected in %d key(s):\n", len(diffs))
	for _, d := range diffs {
		fmt.Printf("  - %s\n", d)
	}
	return fmt.Errorf("metadata drift detected")
}

func runDBHasuraValidate(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	missing, err := database.HasuraValidatePermissions(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("hasura validate: %w", err)
	}

	if len(missing) == 0 {
		fmt.Println("All tracked tables have required permissions (tenant_member, tenant_admin).")
		return nil
	}

	fmt.Printf("Permission coverage issues (%d):\n", len(missing))
	for _, m := range missing {
		fmt.Printf("  - %s\n", m)
	}
	return fmt.Errorf("%d permission coverage issue(s)", len(missing))
}
