package commands

// Purpose: RunE implementations for the "nself db pitr", "nself db backup
// sync", and "nself db restore-drill" subcommands. Inputs are the cobra
// command/args; outputs are printed PITR/backup-sync/restore-drill results or
// an error.
// Constraints: split out of db_pitr.go (CLI-R12) as a pure move, no behavior change.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/database"
	"github.com/spf13/cobra"
)

func runDBPITRStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	status, err := database.GetPITRStatus(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("pitr status: %w", err)
	}

	setData(cmd, dbPITRStatusData{Enabled: status.ArchiveEnabled, ArchiveMode: status.ArchiveMode, WALLevel: status.WALLevel, MaxWALSenders: status.MaxWALSenders, LastArchivedWAL: status.LastArchivedWAL, LastArchiveTime: status.LastArchiveTime})
	enabled := "disabled"
	if status.ArchiveEnabled {
		enabled = "enabled"
	}
	fmt.Printf("PITR:            %s\n", enabled)
	fmt.Printf("archive_mode:    %s\n", status.ArchiveMode)
	fmt.Printf("wal_level:       %s\n", status.WALLevel)
	fmt.Printf("max_wal_senders: %d\n", status.MaxWALSenders)
	fmt.Printf("archive_command: %s\n", status.ArchiveCommand)
	if status.LastArchivedWAL != "" {
		fmt.Printf("last_wal:        %s\n", status.LastArchivedWAL)
		fmt.Printf("last_archived:   %s\n", status.LastArchiveTime)
	}
	return nil
}

func runDBPITREnable(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	archiveDir := "/var/lib/postgresql/wal_archive"
	if cfg.Backup.Remote != "" {
		archiveDir = cfg.Backup.Remote
	}

	pitrCfg := database.PITRConfig{
		ArchiveDir:  archiveDir,
		WalInterval: cfg.Backup.WALInterval,
		RemotePath:  cfg.Backup.Remote,
	}
	if pitrCfg.WalInterval <= 0 {
		pitrCfg.WalInterval = 60
	}

	if err := database.WritePITRConfig(cfg, projectDir, pitrCfg); err != nil {
		return fmt.Errorf("write PITR config: %w", err)
	}

	fmt.Printf("PITR configuration written to %s/.nself/pitr/postgresql.conf.d/pitr.conf\n", projectDir)
	fmt.Println("Mount this file into the PostgreSQL container to enable WAL archiving.")
	return nil
}

func runDBPITRTest(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	fmt.Println("Testing WAL archiving...")
	if err := database.TestWALArchive(cmd.Context(), cfg); err != nil {
		return fmt.Errorf("WAL archive test failed: %w", err)
	}

	fmt.Println("WAL archive test passed.")
	return nil
}

func runDBPITRRestore(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	target, _ := cmd.Flags().GetString("target")
	if target == "" {
		return fmt.Errorf("--target is required (RFC3339 format, e.g. 2024-01-15T14:30:00Z)")
	}

	fmt.Printf("Starting PITR restore to %s...\n", target)
	if err := database.PITRRestore(cmd.Context(), cfg, target); err != nil {
		return fmt.Errorf("PITR restore: %w", err)
	}

	fmt.Printf("PITR restore to %s completed.\n", target)
	return nil
}

func runDBBackupSync(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	remote, _ := cmd.Flags().GetString("remote")
	if remote == "" {
		remote = cfg.Backup.Remote
	}
	if remote == "" {
		return fmt.Errorf("--remote is required or set BACKUP_REMOTE in your .env")
	}

	encrypt, _ := cmd.Flags().GetBool("encrypt")
	recipients, _ := cmd.Flags().GetString("recipients")
	if !encrypt {
		encrypt = cfg.Backup.Encryption
	}
	if recipients == "" {
		recipients = cfg.Backup.AgeRecipients
	}

	retention := cfg.Backup.RetentionDaily
	if retention <= 0 {
		retention = 7
	}

	xrCfg := database.CrossRegionBackupConfig{
		Enabled:         true,
		RemotePath:      remote,
		SecondaryRegion: cfg.DR.SecondaryRegion,
		Retention:       retention,
		Encrypted:       encrypt,
		AgeRecipients:   recipients,
	}

	fmt.Printf("Syncing backups to %s...\n", remote)
	if err := database.SyncBackupToRemote(cmd.Context(), cfg, xrCfg); err != nil {
		return fmt.Errorf("backup sync: %w", err)
	}

	fmt.Println("Backup sync completed.")
	return nil
}

func runDBBackupSyncStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	remote := cfg.Backup.Remote
	if remote == "" {
		return fmt.Errorf("BACKUP_REMOTE is not configured in your .env")
	}

	xrCfg := database.CrossRegionBackupConfig{
		RemotePath: remote,
	}

	status, err := database.GetCrossRegionStatus(cmd.Context(), cfg, xrCfg)
	if err != nil {
		return fmt.Errorf("backup sync status: %w", err)
	}

	syncData := dbSyncStatusData{Remote: remote, Files: status.FilesTransferred, Bytes: status.BytesTransferred, Errors: append([]string{}, status.Errors...)}
	if !status.LastSync.IsZero() {
		t := status.LastSync.UTC().Format(time.RFC3339)
		syncData.LastSync = &t
	}
	setData(cmd, syncData)
	if status.LastSync.IsZero() {
		fmt.Println("No remote backups found.")
		return nil
	}

	fmt.Printf("Remote:           %s\n", remote)
	fmt.Printf("Last sync:        %s\n", status.LastSync.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("Files:            %d\n", status.FilesTransferred)
	fmt.Printf("Total size:       %d bytes\n", status.BytesTransferred)

	if len(status.Errors) > 0 {
		fmt.Printf("Errors:\n")
		for _, e := range status.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	return nil
}

func runDBRestoreDrill(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	backupFile, _ := cmd.Flags().GetString("file")

	fmt.Println("Starting restore drill...")
	result, err := database.RestoreDrill(cmd.Context(), cfg, backupFile)
	if err != nil {
		return fmt.Errorf("restore drill: %w", err)
	}

	status := "PASS"
	if !result.Success {
		status = "FAIL"
	}

	fmt.Printf("Drill status:     %s\n", status)
	fmt.Printf("Backup file:      %s\n", result.BackupFile)
	fmt.Printf("Duration:         %s\n", result.Duration.Round(1e6))
	fmt.Printf("Tables verified:  %d\n", result.TablesVerified)
	fmt.Printf("Rows sampled:     %d\n", result.RowsVerified)

	if result.ErrorMessage != "" {
		fmt.Printf("Error:            %s\n", result.ErrorMessage)
		return fmt.Errorf("restore drill failed")
	}

	return nil
}

func runDBRestoreDrillList(cmd *cobra.Command, _ []string) error {
	drills := dbRestoreDrillListData{Drills: []database.RestoreDrillResult{}}
	setData(cmd, drills)
	logPath := ".nself/restore-drills.log"

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No drill results found. Run 'nself db restore-drill' first.")
			return nil
		}
		return fmt.Errorf("open drill log: %w", err)
	}
	defer func() { _ = f.Close() }()

	fmt.Printf("%-25s %-8s %-40s %-10s %-12s\n", "STARTED", "STATUS", "BACKUP FILE", "TABLES", "DURATION")

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var result database.RestoreDrillResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			continue
		}
		drills.Drills = append(drills.Drills, result)
		setData(cmd, drills)

		status := "PASS"
		if !result.Success {
			status = "FAIL"
		}

		backupBase := result.BackupFile
		if len(backupBase) > 40 {
			backupBase = "..." + backupBase[len(backupBase)-37:]
		}

		fmt.Printf("%-25s %-8s %-40s %-10d %-12s\n",
			result.StartedAt.Format("2006-01-02 15:04:05"),
			status,
			backupBase,
			result.TablesVerified,
			result.Duration.Round(1e6).String(),
		)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read drill log: %w", err)
	}

	return nil
}
