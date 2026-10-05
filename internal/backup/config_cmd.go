package backup

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// ConfigView returns the current backup configuration as a formatted string.
func ConfigView(cfg *config.Config, format string) (string, error) {
	b := cfg.Backup
	view := map[string]interface{}{
		"enabled":               b.Enabled,
		"dir":                   b.Dir,
		"remote":                b.Remote,
		"encryption":            b.Encryption,
		"age_recipients":        b.AgeRecipients,
		"schedule_full":         b.ScheduleFull,
		"wal_interval_seconds":  b.WALInterval,
		"retention_daily":       b.RetentionDaily,
		"retention_weekly":      b.RetentionWeekly,
		"retention_monthly":     b.RetentionMonthly,
		"restore_test_schedule": b.RestoreTestSchedule,
		"alert_on_failure":      b.AlertOnFailure,
		"s3_endpoint":           b.S3Endpoint,
		"s3_region":             b.S3Region,
	}

	if format == "json" {
		data, err := json.MarshalIndent(view, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var sb strings.Builder
	sb.WriteString("Backup Configuration:\n")
	for k, v := range view {
		fmt.Fprintf(&sb, "  %-25s %v\n", k+":", v)
	}
	return sb.String(), nil
}

// StatusInfo holds backup subsystem status.
type StatusInfo struct {
	LastRun          string `json:"last_run"`
	NextRun          string `json:"next_run"`
	Health           string `json:"health"` // healthy, warning, error
	TotalSize        string `json:"total_size"`
	BackupCount      int    `json:"backup_count"`
	RetentionDaily   int    `json:"retention_daily"`
	RetentionWeekly  int    `json:"retention_weekly"`
	RetentionMonthly int    `json:"retention_monthly"`
}

// Status computes and returns the current backup subsystem status.
func Status(cfg *config.Config) (*StatusInfo, error) {
	entries, err := List(cfg, ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	info := &StatusInfo{
		Health:           "healthy",
		BackupCount:      len(entries),
		RetentionDaily:   cfg.Backup.RetentionDaily,
		RetentionWeekly:  cfg.Backup.RetentionWeekly,
		RetentionMonthly: cfg.Backup.RetentionMonthly,
	}

	if len(entries) == 0 {
		info.Health = "warning"
		info.LastRun = "never"
	} else {
		info.LastRun = entries[0].Date.Format("2006-01-02 15:04:05")
	}

	// Compute total size.
	var totalBytes int64
	for _, e := range entries {
		totalBytes += e.Size
	}
	info.TotalSize = formatSize(totalBytes)

	// Determine next run from schedule.
	info.NextRun = cfg.Backup.ScheduleFull
	if info.NextRun == "" {
		info.NextRun = cfg.Backup.Schedule
	}

	if !cfg.Backup.Enabled {
		info.Health = "disabled"
	}

	return info, nil
}

// FormatStatus renders status info as a string.
func FormatStatus(info *StatusInfo, format string) (string, error) {
	if format == "json" {
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var sb strings.Builder
	sb.WriteString("Backup Status:\n")
	fmt.Fprintf(&sb, "  Health:           %s\n", info.Health)
	fmt.Fprintf(&sb, "  Last run:         %s\n", info.LastRun)
	fmt.Fprintf(&sb, "  Next schedule:    %s\n", info.NextRun)
	fmt.Fprintf(&sb, "  Backup count:     %d\n", info.BackupCount)
	fmt.Fprintf(&sb, "  Total size:       %s\n", info.TotalSize)
	fmt.Fprintf(&sb, "  Retention:        daily=%d weekly=%d monthly=%d\n",
		info.RetentionDaily, info.RetentionWeekly, info.RetentionMonthly)
	return sb.String(), nil
}

// statusWithOffbox is StatusInfo plus the offbox object. The embedded fields
// keep their order and names, so the existing JSON is unchanged and `offbox`
// is appended (contract:cli.backup-status v1).
type statusWithOffbox struct {
	*StatusInfo
	Offbox *OffboxStatus `json:"offbox"`
}

// FormatStatusOffbox renders the status with the off-box heartbeat state. info
// is nil outside a project directory (--project with --heartbeat-to): then only
// the offbox object (JSON) or the two off-box lines (text) are printed. The
// text form gains the two lines only when a heartbeat remote is configured.
func FormatStatusOffbox(info *StatusInfo, off *OffboxStatus, format string) (string, error) {
	if format == "json" {
		var v interface{} = struct {
			Offbox *OffboxStatus `json:"offbox"`
		}{off}
		if info != nil {
			v = statusWithOffbox{info, off}
		}
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	var sb strings.Builder
	if info != nil {
		out, err := FormatStatus(info, format)
		if err != nil {
			return "", err
		}
		sb.WriteString(out)
	} else {
		sb.WriteString("Backup Status:\n")
	}
	if off != nil && off.Source == "heartbeat" {
		for _, l := range off.Lines() {
			sb.WriteString(l + "\n")
		}
	}
	return sb.String(), nil
}
