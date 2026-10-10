package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/database"
	"github.com/spf13/cobra"
)

// ── hasura git-aware workflow commands ──────────────────────────────────────

var dbHasuraDriftCmd = &cobra.Command{
	Use:   "metadata-drift",
	Short: "Detect drift between live Hasura metadata and committed files",
	Long: `Compare live Hasura metadata against the committed YAML files in
hasura/metadata/databases/default/tables/.

Reports tables that are tracked in live Hasura but absent from committed files
(added) and tables present on disk but no longer tracked in live Hasura
(removed).`,
	RunE: dataEnv(runDBHasuraDrift),
}

var dbHasuraSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Export live Hasura metadata and commit to git",
	Long: `Export current Hasura metadata to hasura/metadata/ and create a git commit.

Use --message to supply a custom commit message. If omitted, a timestamped
default is generated.`,
	RunE: dataEnv(runDBHasuraSync),
}

var dbHasuraGitStatusCmd = &cobra.Command{
	Use:   "git-status",
	Short: "Show git status of the hasura/metadata/ directory",
	RunE:  dataEnv(runDBHasuraGitStatus),
}

var dbHasuraApplyRefCmd = &cobra.Command{
	Use:   "apply-ref <ref>",
	Short: "Apply Hasura metadata from a specific git ref",
	Long: `Check out hasura/metadata/ at the given git ref (branch, tag, or commit hash)
and apply it to the running Hasura instance.`,
	Args: cobra.ExactArgs(1),
	RunE: dataEnv(runDBHasuraApplyRef),
}

var dbHasuraSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Capture or compare Hasura metadata snapshots",
	Long: `Capture the current live Hasura metadata as a JSON snapshot.

Without --compare, prints the snapshot JSON to stdout.
With --compare <file>, compares the captured snapshot against the file and
prints a human-readable diff.`,
	RunE: dataEnv(runDBHasuraSnapshot),
}

func init() {
	dbHasuraCmd.AddCommand(dbHasuraDriftCmd)
	dbHasuraCmd.AddCommand(dbHasuraSyncCmd)
	dbHasuraCmd.AddCommand(dbHasuraGitStatusCmd)
	dbHasuraCmd.AddCommand(dbHasuraApplyRefCmd)
	dbHasuraCmd.AddCommand(dbHasuraSnapshotCmd)

	dbHasuraSyncCmd.Flags().String("message", "", "commit message")
	addDBRemoteFlags(dbHasuraSyncCmd)
	addDBRemoteFlags(dbHasuraApplyRefCmd)
	dbHasuraApplyRefCmd.Flags().Bool("yes", false, "Confirm a prod-class metadata plan")
	dbHasuraSnapshotCmd.Flags().String("compare", "", "path to a previous snapshot file to diff against")
}

func runDBHasuraDrift(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	drift, err := database.DetectMetadataDrift(cmd.Context(), cfg, dir)
	if err != nil {
		return fmt.Errorf("detecting metadata drift: %w", err)
	}

	fmt.Println("Hasura metadata drift:")

	if len(drift.TablesAdded) == 0 {
		fmt.Println("  Tables added (live not in files):    (none)")
	} else {
		fmt.Printf("  Tables added (live not in files):    %s\n", strings.Join(drift.TablesAdded, ", "))
	}

	if len(drift.TablesRemoved) == 0 {
		fmt.Println("  Tables removed (in files not live):  (none)")
	} else {
		fmt.Printf("  Tables removed (in files not live):  %s\n", strings.Join(drift.TablesRemoved, ", "))
	}

	if len(drift.PermissionsChanged) > 0 {
		fmt.Printf("  Permissions changed:                 %s\n", strings.Join(drift.PermissionsChanged, ", "))
	}

	if drift.IsClean {
		fmt.Println("  Status: CLEAN (live metadata matches committed files)")
		return nil
	}

	fmt.Println("  Status: DRIFT DETECTED (run: nself db hasura sync to commit live state)")
	return fmt.Errorf("metadata drift detected")
}

func runDBHasuraSync(cmd *cobra.Command, _ []string) error {
	rt, err := resolveDBRemoteTarget(cmd)
	if err != nil {
		return err
	}
	if !rt.Local {
		return runDBHasuraRemoteSync(cmd, rt)
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	msg, err := cmd.Flags().GetString("message")
	if err != nil {
		return fmt.Errorf("reading --message flag: %w", err)
	}

	hash, err := database.ExportAndCommitMetadata(cmd.Context(), cfg, dir, msg)
	if err != nil {
		return fmt.Errorf("syncing metadata: %w", err)
	}

	fmt.Printf("Metadata exported and committed: %s\n", hash)
	return nil
}

func runDBHasuraGitStatus(cmd *cobra.Command, _ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	status, err := database.GetGitStatus(dir)
	if err != nil {
		return fmt.Errorf("getting git status: %w", err)
	}

	modified := status.Modified
	if modified == nil {
		modified = []string{}
	}
	setData(cmd, dbGitStatusData{Branch: status.Branch, Commit: status.CommitHash, Modified: modified, Clean: status.IsClean})
	fmt.Printf("Branch: %s\n", status.Branch)
	fmt.Printf("Commit: %s\n", status.CommitHash)

	if len(status.Modified) == 0 {
		fmt.Println("Modified metadata files: (none)")
	} else {
		fmt.Println("Modified metadata files:")
		for _, f := range status.Modified {
			fmt.Printf("  %s\n", f)
		}
	}

	if status.IsClean {
		fmt.Println("Status: CLEAN")
	} else {
		fmt.Println("Status: DIRTY (uncommitted changes)")
	}
	return nil
}

func runDBHasuraApplyRef(cmd *cobra.Command, args []string) error {
	rt, err := resolveDBRemoteTarget(cmd)
	if err != nil {
		return err
	}
	if !rt.Local {
		return runDBHasuraRemoteApplyRef(cmd, rt, args[0])
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	ref := args[0]
	if err := database.ApplyMetadataFromGit(cmd.Context(), cfg, dir, ref); err != nil {
		return fmt.Errorf("applying metadata from ref %s: %w", ref, err)
	}

	fmt.Printf("Applied Hasura metadata from git ref: %s\n", ref)
	return nil
}

func runDBHasuraSnapshot(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	compareFile, err := cmd.Flags().GetString("compare")
	if err != nil {
		return fmt.Errorf("reading --compare flag: %w", err)
	}

	snapshot, err := database.MetadataSnapshot(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("capturing metadata snapshot: %w", err)
	}

	if compareFile == "" {
		fmt.Println(string(snapshot))
		return nil
	}

	before, err := os.ReadFile(compareFile)
	if err != nil {
		return fmt.Errorf("reading compare file %s: %w", compareFile, err)
	}

	diff, err := database.CompareSnapshots(before, snapshot)
	if err != nil {
		return fmt.Errorf("comparing snapshots: %w", err)
	}

	fmt.Println(diff)
	return nil
}
