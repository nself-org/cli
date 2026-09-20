package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// BackupEntry holds parsed metadata for a single backup file.
type BackupEntry struct {
	ID        string    `json:"id"`
	Date      time.Time `json:"date"`
	Size      int64     `json:"size"`
	Type      string    `json:"type"`      // full, metadata, minio, wal
	Tag       string    `json:"tag"`       // user tag if present
	Encrypted bool      `json:"encrypted"` // .age suffix
}

// ListOptions holds flags for `nself backup list`.
type ListOptions struct {
	Remote string        // filter by remote name
	Env    string        // filter by environment
	Since  time.Duration // only show backups newer than this
	Format string        // table or json
}

// List returns backup entries from the local backup directory.
func List(cfg *config.Config, opts ListOptions) ([]BackupEntry, error) {
	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "./backups"
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. No backup directory means no backups — `nself backup list`
			// on a project that has never run one should print an empty list,
			// not an error. ENOENT only; a backup dir that exists and cannot
			// be read errors, because "you have no backups" is a dangerous
			// thing to tell someone about to overwrite something.
			return []BackupEntry{}, nil
		}
		return nil, fmt.Errorf("read backup directory %s: %w", backupDir, err)
	}

	var backups []BackupEntry
	cutoff := time.Time{}
	if opts.Since > 0 {
		cutoff = time.Now().Add(-opts.Since)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if !cutoff.IsZero() && info.ModTime().Before(cutoff) {
			continue
		}

		be := BackupEntry{
			ID:        strings.TrimSuffix(strings.TrimSuffix(name, ".age"), filepath.Ext(strings.TrimSuffix(name, ".age"))),
			Date:      info.ModTime(),
			Size:      info.Size(),
			Type:      inferBackupType(name),
			Encrypted: strings.HasSuffix(name, ".age"),
		}

		// Extract tag if present (format: project_type_timestamp_tag.ext).
		parts := strings.Split(be.ID, "_")
		if len(parts) > 3 {
			be.Tag = strings.Join(parts[3:], "_")
		}

		backups = append(backups, be)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Date.After(backups[j].Date)
	})

	return backups, nil
}

// FormatList renders backup entries as a table or JSON string.
func FormatList(backups []BackupEntry, format string) (string, error) {
	if format == "json" {
		data, err := json.MarshalIndent(backups, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	// Table output.
	var sb strings.Builder
	fmt.Fprintf(&sb, "%-40s  %-21s  %-8s  %-10s  %s\n", "ID", "DATE", "SIZE", "TYPE", "ENCRYPTED")
	for _, b := range backups {
		enc := ""
		if b.Encrypted {
			enc = "yes"
		}
		fmt.Fprintf(&sb, "%-40s  %-21s  %-8s  %-10s  %s\n",
			b.ID,
			b.Date.Format("2006-01-02 15:04:05"),
			formatSize(b.Size),
			b.Type,
			enc,
		)
	}
	return sb.String(), nil
}

func inferBackupType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "_full_"):
		return "full"
	case strings.Contains(lower, "_metadata_"):
		return "metadata"
	case strings.Contains(lower, "minio_"):
		return "minio"
	case strings.Contains(lower, "_wal_") || strings.HasSuffix(lower, ".wal"):
		return "wal"
	default:
		return "manual"
	}
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.1fGB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.0fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
