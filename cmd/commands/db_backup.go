package commands

// Purpose: RunE implementations for "nself db backup/restore/backup list" plus
// the backupEntry type and the formatBackupSize helper. Inputs are the cobra
// command/args; outputs are backup results printed to the user or an error.
// Constraints: split out of db.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/database"

	"github.com/spf13/cobra"
)

func runDBBackup(cmd *cobra.Command, args []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	var outputPath string
	if len(args) > 0 {
		outputPath = args[0]
	}
	if err := database.Backup(cmd.Context(), cfg, outputPath); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	fmt.Println("Backup created successfully.")
	return nil
}

func runDBRestore(cmd *cobra.Command, args []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	overwrite, _ := cmd.Flags().GetBool("overwrite")
	yesFlag, _ := cmd.Flags().GetBool("yes")
	if overwrite && cfg.IsProduction() && !yesFlag {
		if err := requireProductionConfirmation(cfg.ProjectName); err != nil {
			return err
		}
	}
	if err := database.Restore(cmd.Context(), cfg, args[0]); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	fmt.Println("Database restored successfully.")
	return nil
}

// backupEntry holds parsed metadata for a single backup file.
type backupEntry struct {
	ID   string    `json:"id"`
	Date time.Time `json:"date"`
	Size int64     `json:"size"`
	Type string    `json:"type"`
}

func runDBBackupList(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "backups"
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			if jsonEnvelopeOn(cmd) {
				return emitEnv(cmd, []backupEntry{}, true)
			}
			fmt.Println("No backups found.")
			return nil
		}
		return fmt.Errorf("reading backup directory %s: %w", backupDir, err)
	}

	var backups []backupEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Accept .dump and .sql files only.
		if !strings.HasSuffix(name, ".dump") && !strings.HasSuffix(name, ".sql") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Derive backup type from filename: files containing "scheduled" or
		// created outside the manual CLI path are labelled "scheduled";
		// everything else is "manual".
		backupType := "manual"
		if strings.Contains(strings.ToLower(name), "scheduled") ||
			strings.Contains(strings.ToLower(name), "auto") {
			backupType = "scheduled"
		}

		// Use the file modification time as the backup date.
		id := strings.TrimSuffix(strings.TrimSuffix(name, ".dump"), ".sql")
		backups = append(backups, backupEntry{
			ID:   id,
			Date: info.ModTime(),
			Size: info.Size(),
			Type: backupType,
		})
	}

	if jsonEnvelopeOn(cmd) {
		if backups == nil {
			backups = []backupEntry{}
		}
		return emitEnv(cmd, backups, true)
	}
	if len(backups) == 0 {
		fmt.Println("No backups found.")
		return nil
	}

	format, _ := cmd.Flags().GetString("format")
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(backups)
	}

	// Table output.
	fmt.Printf("%-30s  %-21s  %-8s  %s\n", "ID", "DATE", "SIZE", "TYPE")
	for _, b := range backups {
		sizeStr := formatBackupSize(b.Size)
		fmt.Printf("%-30s  %-21s  %-8s  %s\n",
			b.ID,
			b.Date.Format("2006-01-02 15:04:05"),
			sizeStr,
			b.Type,
		)
	}
	return nil
}

// formatBackupSize returns a human-readable size string (KB, MB, GB).
func formatBackupSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.0fGB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.0fMB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.0fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
