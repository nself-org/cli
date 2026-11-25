// Package contract holds contract:data.lifecycle.v1, the data durability
// contract a consumer reads from an nSelf stack (P8 Part C, DATA).
//
// Purpose: one published shape for the backup catalog, the retention policy,
// purge status and pressure telemetry, so an external app or dev agent can
// rely on them without reading nSelf internals.
// Inputs: none; this package is types plus the embedded JSON Schema.
// Outputs: Go types whose JSON form validates against schema.json, and
// SchemaJSON() for publishing the schema on CLI, HTTP and MCP surfaces.
// Constraints: stdlib only, no I/O. The version lives in the schema $id
// (SchemaID); documents carry no version field. There are no sync types:
// the contract describes one stack's state, never a bidirectional sync.
// Additive changes only; a breaking change is data.lifecycle.v2 beside v1.
// The on-disk backup formats (internal/backup, internal/backup/pitr) are not
// changed by this contract; producers adapt their records to these types.
package contract

import (
	_ "embed"
	"time"
)

// ContractID is the SPORT contract name this package provides.
const ContractID = "contract:data.lifecycle.v1"

// SchemaID is the $id of schema.json. The trailing ":v1" is the version.
const SchemaID = "urn:nself:cli:schema:data-lifecycle:v1"

//go:embed schema.json
var schemaJSON []byte

// SchemaJSON returns a copy of the published JSON Schema (draft 2020-12).
func SchemaJSON() []byte {
	out := make([]byte, len(schemaJSON))
	copy(out, schemaJSON)
	return out
}

// Document is the top-level schema object. Every section is optional, so a
// surface can return only the part it was asked for.
type Document struct {
	BackupCatalog *BackupCatalog     `json:"backup_catalog,omitempty"`
	Retention     *RetentionPolicy   `json:"retention,omitempty"`
	Purges        []PurgeStatus      `json:"purges,omitempty"`
	Pressure      *PressureTelemetry `json:"pressure,omitempty"`
}

// BackupKind names what a backup holds.
type BackupKind string

// Backup kinds.
const (
	BackupFull     BackupKind = "full"
	BackupDatabase BackupKind = "database"
	BackupStorage  BackupKind = "storage"
	BackupConfig   BackupKind = "config"
	BackupPITRBase BackupKind = "pitr_base"
)

// BackupCatalog lists every backup of one stack and when each expires.
type BackupCatalog struct {
	Stack       string        `json:"stack"`
	GeneratedAt time.Time     `json:"generated_at"`
	Backups     []BackupEntry `json:"backups"`
}

// BackupEntry is one backup in the catalog. ExpiresAt is nil when retention
// keeps the backup with no fixed expiry (for example a kept tag).
type BackupEntry struct {
	ID             string       `json:"id"`
	Kind           BackupKind   `json:"kind"`
	CreatedAt      time.Time    `json:"created_at"`
	ExpiresAt      *time.Time   `json:"expires_at,omitempty"`
	SizeBytes      int64        `json:"size_bytes"`
	ChecksumSHA256 string       `json:"checksum_sha256"`
	Encryption     Encryption   `json:"encryption"`
	Location       Location     `json:"location"`
	Verification   Verification `json:"verification"`
	Tags           []string     `json:"tags,omitempty"`
}

// Encryption states whether a backup is encrypted at rest and which key
// custody reference opens it. KeyRef is a credential reference, never key
// material.
type Encryption struct {
	Encrypted bool   `json:"encrypted"`
	Algorithm string `json:"algorithm,omitempty"`
	KeyRef    string `json:"key_ref,omitempty"`
}

// Location says where a backup is stored: a destination name (local, s3,
// r2 and so on) and a reference inside it (path or object key).
type Location struct {
	Destination string `json:"destination"`
	Ref         string `json:"ref"`
}

// VerifyStatus is the result of the latest backup verification.
type VerifyStatus string

// Verification statuses.
const (
	VerifyUnverified VerifyStatus = "unverified"
	VerifyPassed     VerifyStatus = "passed"
	VerifyFailed     VerifyStatus = "failed"
)

// VerifyMethod is how a backup was verified: a checksum check, or a restore
// into a scratch instance with row counts and checksums.
type VerifyMethod string

// Verification methods.
const (
	VerifyChecksum VerifyMethod = "checksum"
	VerifyRestore  VerifyMethod = "restore"
)

// Verification records the latest verification of a backup.
type Verification struct {
	Status     VerifyStatus `json:"status"`
	Method     VerifyMethod `json:"method,omitempty"`
	VerifiedAt *time.Time   `json:"verified_at,omitempty"`
	Detail     string       `json:"detail,omitempty"`
}

// RetentionPolicy is the stack's backup retention. The keep_* counts match
// the existing backup prune options; zero means "keep none in that bucket".
type RetentionPolicy struct {
	KeepLast    int  `json:"keep_last,omitempty"`
	KeepDaily   int  `json:"keep_daily"`
	KeepWeekly  int  `json:"keep_weekly"`
	KeepMonthly int  `json:"keep_monthly"`
	MaxAgeDays  int  `json:"max_age_days,omitempty"`
	KeepTagged  bool `json:"keep_tagged,omitempty"`
}

// PurgeSubjectKind names what a purge request deletes.
type PurgeSubjectKind string

// Purge subject kinds.
const (
	PurgeRow           PurgeSubjectKind = "row"
	PurgeObject        PurgeSubjectKind = "object"
	PurgeIndexDocument PurgeSubjectKind = "index_document"
	PurgeWorkspace     PurgeSubjectKind = "workspace"
)

// PurgeSubject identifies the purged thing inside the stack.
type PurgeSubject struct {
	Kind PurgeSubjectKind `json:"kind"`
	Ref  string           `json:"ref"`
}

// PurgeStatus keeps the four purge facts separate, so nSelf never claims an
// instant global deletion: archived, deleted in the primary store, deleted in
// derived indexes, and the latest expiry of a backup that still holds the
// subject (PendingBackupExpiry, null when no backup holds it).
type PurgeStatus struct {
	Subject             PurgeSubject `json:"subject"`
	RequestedAt         time.Time    `json:"requested_at"`
	TombstoneID         string       `json:"tombstone_id,omitempty"`
	Archived            bool         `json:"archived"`
	DeletedInPrimary    bool         `json:"deleted_in_primary"`
	DeletedInIndexes    bool         `json:"deleted_in_indexes"`
	PendingBackupExpiry *time.Time   `json:"pending_backup_expiry"`
}

// Complete reports whether the subject is gone everywhere nSelf controls:
// the primary store, the derived indexes, and every backup (no pending
// backup expiry). Archived alone is not deletion.
func (p PurgeStatus) Complete() bool {
	return p.DeletedInPrimary && p.DeletedInIndexes && p.PendingBackupExpiry == nil
}

// Engine names the primary database engine of a stack.
type Engine string

// Database engines.
const (
	EnginePostgres Engine = "postgres"
	EngineSQLite   Engine = "sqlite"
)

// PressureTelemetry is a point-in-time pressure reading, served through the
// existing health/metrics surface, so an app can recommend tuning before it
// recommends moving to Postgres.
type PressureTelemetry struct {
	Stack           string       `json:"stack"`
	Engine          Engine       `json:"engine"`
	ObservedAt      time.Time    `json:"observed_at"`
	WriteLockWaitMS float64      `json:"write_lock_wait_ms"`
	QueueAgeSeconds float64      `json:"queue_age_seconds"`
	QueryLatencyMS  Latency      `json:"query_latency_ms"`
	Connections     Connections  `json:"connections"`
	Storage         StorageUsage `json:"storage"`
	BackupHealth    BackupHealth `json:"backup_health"`
}

// Latency holds query latency percentiles in milliseconds.
type Latency struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

// Connections is connection use against the configured maximum.
type Connections struct {
	InUse int `json:"in_use"`
	Max   int `json:"max"`
}

// StorageUsage is bytes used and, when known, the capacity.
type StorageUsage struct {
	UsedBytes     int64 `json:"used_bytes"`
	CapacityBytes int64 `json:"capacity_bytes,omitempty"`
}

// BackupHealthStatus summarises backup health for pressure readings.
type BackupHealthStatus string

// Backup health statuses.
const (
	BackupHealthOK      BackupHealthStatus = "ok"
	BackupHealthStale   BackupHealthStatus = "stale"
	BackupHealthFailing BackupHealthStatus = "failing"
	BackupHealthNone    BackupHealthStatus = "none"
)

// BackupHealth is the backup state shown with pressure telemetry.
type BackupHealth struct {
	Status        BackupHealthStatus `json:"status"`
	LastSuccessAt *time.Time         `json:"last_success_at,omitempty"`
}
