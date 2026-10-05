package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
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

// listRemoteTimeout bounds one remote listing.
const listRemoteTimeout = 2 * time.Minute

// List returns backup entries from the local backup directory, or from the
// destination named by opts.Remote (rclone remote, path:// or host://).
func List(cfg *config.Config, opts ListOptions) ([]BackupEntry, error) {
	if opts.Remote != "" {
		return listRemote(cfg, opts)
	}
	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "./backups"
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
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

		backups = append(backups, newBackupEntry(name, info.Size(), info.ModTime()))
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Date.After(backups[j].Date)
	})

	return backups, nil
}

// newBackupEntry builds the entry for one backup file name.
func newBackupEntry(name string, size int64, mod time.Time) BackupEntry {
	be := BackupEntry{
		ID:        strings.TrimSuffix(strings.TrimSuffix(name, ".age"), filepath.Ext(strings.TrimSuffix(name, ".age"))),
		Date:      mod,
		Size:      size,
		Type:      inferBackupType(name),
		Encrypted: strings.HasSuffix(name, ".age"),
	}
	// Extract tag if present (format: project_type_timestamp_tag.ext).
	parts := strings.Split(be.ID, "_")
	if len(parts) > 3 {
		be.Tag = strings.Join(parts[3:], "_")
	}
	return be
}

// listRemote lists backups at opts.Remote, newest first.
func listRemote(cfg *config.Config, opts ListOptions) ([]BackupEntry, error) {
	env := rcloneEnvFor(cfg)
	dest, err := destinationFor(opts.Remote, env...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrBackupRemoteFailed, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), listRemoteTimeout)
	defer cancel()
	objs, err := dest.List(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrBackupRemoteFailed, err)
	}
	cutoff := time.Time{}
	if opts.Since > 0 {
		cutoff = time.Now().Add(-opts.Since)
	}
	backups := make([]BackupEntry, 0, len(objs))
	for _, o := range objs {
		if !cutoff.IsZero() && o.ModTime.Before(cutoff) {
			continue
		}
		backups = append(backups, newBackupEntry(filepath.Base(o.Key), o.Size, o.ModTime))
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Date.After(backups[j].Date) })
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
