package commands

// Purpose: typed envelope data of the data fragment's read commands
// (P7-SURF-14). Each type holds the facts the command's human output shows;
// no row data, no child-process output. Field names are snake_case and stable.
// SPORT: P7-SURF-14

import (
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/tenant"
)

type dbMigrationRow struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"` // applied | pending
	AppliedAt *string `json:"applied_at"`
}

type dbMigrateStatusData struct {
	Migrations []dbMigrationRow `json:"migrations"`
}

type dbMigrationAuditRow struct {
	Name          string   `json:"name"`
	Applied       bool     `json:"applied"`
	Idempotent    bool     `json:"idempotent"`
	HasRollback   bool     `json:"has_rollback"`
	ChecksumMatch bool     `json:"checksum_match"`
	Issues        []string `json:"issues"`
}

type dbMigrateAuditData struct {
	Migrations []dbMigrationAuditRow `json:"migrations"`
}

type dbMigrateLintData struct {
	Files []string `json:"files"` // every file passed lint (a failure is an error)
}

type dbDriftTableRow struct {
	Schema  string   `json:"schema"`
	Table   string   `json:"table"`
	Present []string `json:"present_columns"`
	Missing []string `json:"missing_columns"`
	Score   int      `json:"drift_score"`
}

type dbDriftScanData struct {
	Tables          []dbDriftTableRow `json:"tables"`
	TotalTables     int               `json:"total_tables"`
	CompliantTables int               `json:"compliant_tables"`
	DriftedTables   int               `json:"drifted_tables"`
	OverallScore    int               `json:"overall_score"`
}

type dbVerifyData struct {
	Role      string `json:"role"`
	Queries   int    `json:"queries"`
	Mutations int    `json:"mutations"`
}

type dbReconcileChange struct {
	Table       string `json:"table"`
	Description string `json:"description"`
}

type dbReconcileData struct {
	Changes []dbReconcileChange `json:"changes"`
	Applied bool                `json:"applied"`
}

type dbGitStatusData struct {
	Branch   string   `json:"branch"`
	Commit   string   `json:"commit"`
	Modified []string `json:"modified"`
	Clean    bool     `json:"clean"`
}

type dbConsoleData struct {
	URL string `json:"url"`
}

type dbSoftDeleteRow struct {
	Schema       string `json:"schema"`
	Table        string `json:"table"`
	HasDeletedAt bool   `json:"has_deleted_at"`
	HasIndex     bool   `json:"has_index"`
	HasView      bool   `json:"has_view"`
}

type dbSoftDeleteAuditData struct {
	Tables []dbSoftDeleteRow `json:"tables"`
}

type dbFKIndexRow struct {
	Schema        string `json:"schema"`
	Table         string `json:"table"`
	Column        string `json:"column"`
	ForeignSchema string `json:"foreign_schema"`
	ForeignTable  string `json:"foreign_table"`
	HasIndex      bool   `json:"has_index"`
}

type dbFKIndexAuditData struct {
	Columns []dbFKIndexRow `json:"columns"`
}

type dbPITRStatusData struct {
	Enabled         bool   `json:"enabled"`
	ArchiveMode     string `json:"archive_mode"`
	WALLevel        string `json:"wal_level"`
	MaxWALSenders   int    `json:"max_wal_senders"`
	LastArchivedWAL string `json:"last_archived_wal"`
	LastArchiveTime string `json:"last_archive_time"`
}

type dbPgBouncerStatusData struct {
	Enabled       bool   `json:"enabled"`
	Running       bool   `json:"running"`
	PoolMode      string `json:"pool_mode"`
	ActivePools   int    `json:"active_pools"`
	ActiveClients int    `json:"active_clients"`
	IdleServers   int    `json:"idle_servers"`
}

type dbSyncStatusData struct {
	Remote   string   `json:"remote"`
	LastSync *string  `json:"last_sync"`
	Files    int      `json:"files"`
	Bytes    int64    `json:"bytes"`
	Errors   []string `json:"errors"`
}

type dbRestoreDrillListData struct {
	Drills []database.RestoreDrillResult `json:"drills"`
}

type dbSeedRow struct {
	Name      string   `json:"name"`
	Env       string   `json:"env"`
	Type      string   `json:"type"` // standard | idempotent | destructive
	DependsOn []string `json:"depends_on"`
}

type dbSeedListData struct {
	Seeds    []dbSeedRow `json:"seeds"`
	Fixtures []string    `json:"fixtures"`
}

type dbSeedGraphNode struct {
	Name      string   `json:"name"`
	Env       string   `json:"env"`
	DependsOn []string `json:"depends_on"`
}

type dbSeedGraphData struct {
	Nodes []dbSeedGraphNode `json:"nodes"`
}

type dbSeedVerifyData struct {
	Fixture string      `json:"fixture"`
	Seeds   []dbSeedRow `json:"seeds"`
}

// dbLintData is the `db lint` document: the tenant_id check (tables) or, with
// --rls, the full report. The other member is null.
type dbLintData struct {
	Tables []tenant.LintResult   `json:"tables"`
	Report *tenant.LintRLSReport `json:"report"`
}

// backupStatusData is the `backup status` document (contract:cli.backup-status
// v1): the local StatusInfo fields, unchanged and in their order, then offbox.
// Outside a project (--project with --heartbeat-to) only offbox is present.
type backupStatusData struct {
	LastRun          *string       `json:"last_run,omitempty"`
	NextRun          *string       `json:"next_run,omitempty"`
	Health           *string       `json:"health,omitempty"`
	TotalSize        *string       `json:"total_size,omitempty"`
	BackupCount      *int          `json:"backup_count,omitempty"`
	RetentionDaily   *int          `json:"retention_daily,omitempty"`
	RetentionWeekly  *int          `json:"retention_weekly,omitempty"`
	RetentionMonthly *int          `json:"retention_monthly,omitempty"`
	Offbox           *backupOffbox `json:"offbox"`
}

// backupOffbox is backup.OffboxStatus (contract:cli.backup-status v1) with the
// nullable-map heartbeat type.
type backupOffbox struct {
	Source              string          `json:"source"`
	LastBackup          *drillHeartbeat `json:"last_backup"`
	LastDrill           *drillHeartbeat `json:"last_drill"`
	BackupAgeSeconds    *int64          `json:"backup_age_seconds"`
	DrillAgeSeconds     *int64          `json:"drill_age_seconds"`
	MaxAgeExceeded      bool            `json:"max_age_exceeded"`
	MaxDrillAgeExceeded bool            `json:"max_drill_age_exceeded"`
	Problems            []string        `json:"problems"`
}

// backupDrillData is the `backup drill` document: the local drill result, or
// with --from the drill heartbeat object. The other member is null.
type backupDrillData struct {
	Local  *database.DrillResult `json:"local"`
	Remote *drillHeartbeat       `json:"remote"`
}

// drillHeartbeat is backup.Heartbeat with nullable row maps, so a backup
// object (restored_rows null) and a drill object both fit one schema.
type drillHeartbeat struct {
	ApproxRows    *map[string]int64 `json:"approx_rows"`
	At            string            `json:"at"`
	BackupKey     string            `json:"backup_key"`
	Bytes         int64             `json:"bytes"`
	CLIVersion    string            `json:"cli_version"`
	Encrypted     bool              `json:"encrypted"`
	Kind          string            `json:"kind"`
	Mismatches    []string          `json:"mismatches"`
	Project       string            `json:"project"`
	RestoredRows  *map[string]int64 `json:"restored_rows"`
	Result        string            `json:"result"`
	SchemaVersion string            `json:"schema_version"`
}

// templateBundledRow is one embedded clone template.
type templateBundledRow struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Plugins []string `json:"plugins"`
}

// templateListData is the `init template list` document.
type templateListData struct {
	Bundled   []templateBundledRow `json:"bundled"`
	Community []templateEntry      `json:"community"`
}

type backupPITRStatusData struct {
	BaseBackups   int     `json:"base_backups"`
	WALSegments   int     `json:"wal_segments"`
	OldestRestore *string `json:"oldest_restore"`
	LatestWAL     *string `json:"latest_wal"`
}
