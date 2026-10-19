package backup

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// PruneOptions holds flags for `nself backup prune`.
type PruneOptions struct {
	DryRun      bool
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
}

// isKeepTag reports whether a backup tag holds the backup out of retention.
// Inputs: the tag parsed from the file name. Outputs: true for "keep" and
// "keep-*". Constraints: pure; "keeper" and other look-alikes are not holds.
func isKeepTag(tag string) bool { return tag == "keep" || strings.HasPrefix(tag, "keep-") }

// PruneResult summarizes what was (or would be) pruned.
type PruneResult struct {
	Kept   []string
	Pruned []string
	DryRun bool
}

// Prune removes old backups according to the retention policy.
// Retention: keep last N daily, N weekly (Sundays), N monthly (1st of month).
// Backups tagged "keep", or "keep-<anything>" (a caller-owned retention hold
// such as the update-stack run backup), are never deleted.
// WAL segments newer than the oldest surviving base backup are never deleted.
func Prune(cfg *config.Config, opts PruneOptions) (*PruneResult, error) {
	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "./backups"
	}

	keepDaily := opts.KeepDaily
	if keepDaily == 0 {
		keepDaily = cfg.Backup.RetentionDaily
	}
	keepWeekly := opts.KeepWeekly
	if keepWeekly == 0 {
		keepWeekly = cfg.Backup.RetentionWeekly
	}
	keepMonthly := opts.KeepMonthly
	if keepMonthly == 0 {
		keepMonthly = cfg.Backup.RetentionMonthly
	}

	entries, err := List(cfg, ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list backups for pruning: %w", err)
	}

	// Only files this package writes are prunable; anything else in the directory
	// (.gitkeep, notes, a restore temp file) is not a backup and is left alone.
	entries = backupFilesOnly(entries)

	// Sort newest first.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date.After(entries[j].Date)
	})

	keep := make(map[string]bool)
	result := &PruneResult{DryRun: opts.DryRun}

	// Track daily/weekly/monthly slots.
	dailySlots := make(map[string]int)   // "2006-01-02" -> count
	weeklySlots := make(map[string]int)  // "2006-W01" -> count
	monthlySlots := make(map[string]int) // "2006-01" -> count

	for _, e := range entries {
		// Tagged "keep" or "keep-..." are always preserved.
		if isKeepTag(e.Tag) {
			keep[e.ID] = true
			continue
		}

		dayKey := e.Date.Format("2006-01-02")
		_, week := e.Date.ISOWeek()
		weekKey := fmt.Sprintf("%d-W%02d", e.Date.Year(), week)
		monthKey := e.Date.Format("2006-01")

		kept := false

		// Daily retention.
		if dailySlots[dayKey] < 1 && len(dailySlots) < keepDaily {
			dailySlots[dayKey]++
			kept = true
		}

		// Weekly retention (Sundays).
		if e.Date.Weekday() == time.Sunday && weeklySlots[weekKey] < 1 && len(weeklySlots) < keepWeekly {
			weeklySlots[weekKey]++
			kept = true
		}

		// Monthly retention (1st of month).
		if e.Date.Day() == 1 && monthlySlots[monthKey] < 1 && len(monthlySlots) < keepMonthly {
			monthlySlots[monthKey]++
			kept = true
		}

		if kept {
			keep[e.ID] = true
		}
	}

	// Find oldest surviving base backup for WAL protection.
	var oldestKeptBase time.Time
	for _, e := range entries {
		if keep[e.ID] && (e.Type == "full" || e.Type == "manual") {
			oldestKeptBase = e.Date
		}
	}

	// Apply pruning.
	for _, e := range entries {
		if keep[e.ID] {
			result.Kept = append(result.Kept, e.ID)
			continue
		}

		// Protect WAL segments newer than oldest base backup.
		if e.Type == "wal" && !oldestKeptBase.IsZero() && e.Date.After(oldestKeptBase) {
			result.Kept = append(result.Kept, e.ID)
			continue
		}

		result.Pruned = append(result.Pruned, e.ID)

		if !opts.DryRun {
			// Remove the entry's own file: the ID has no extension.
			path := filepath.Join(backupDir, e.File)
			if err := os.Remove(path); err != nil {
				slog.Warn("failed to remove backup", "path", path, "error", err)
			} else {
				slog.Info("pruned backup", "path", path)
			}
		}
	}

	if len(result.Pruned) == 0 {
		slog.Info("no backups to prune")
	} else if opts.DryRun {
		slog.Info("dry-run: would prune backups", "count", len(result.Pruned))
	} else {
		slog.Info("pruned backups", "count", len(result.Pruned))
	}

	return result, nil
}

// backupPayloadExts are the extensions create, stream and WAL archiving write.
var backupPayloadExts = []string{".dump", ".tar.gz", ".sql", ".wal"}

// backupFilesOnly keeps the entries whose file name is a backup this package
// writes: a payload extension, optionally followed by ".age".
func backupFilesOnly(entries []BackupEntry) []BackupEntry {
	out := entries[:0:0]
	for _, e := range entries {
		name := strings.TrimSuffix(e.File, ".age")
		for _, ext := range backupPayloadExts {
			if strings.HasSuffix(name, ext) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}
