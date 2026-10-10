// JSON data-type registration for the data domain (db, backup, init template,
// update project).
//
// Purpose: every runnable document command of the data canon fragment answers
// with the v1 envelope in v1.5 mode (P7-SURF-14). Command groups answer with
// their subcommands (dataHub); read commands with typed facts
// (db_envelope_types.go); write, remote and destructive commands, and reads
// whose facts only a child process holds, with the shared dataResult.
// Commands that printed JSON before the contract keep that output in v1.4;
// all rows are v1.5-only envelopes. `backup stream`, `db migrate watch` and
// `db shell` are stream or interactive and register nothing.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import (
	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/database"
)

// dataEnvelopeTypes maps each path of the data fragment to its envelope data.
var dataEnvelopeTypes = map[string]any{
	"backup":                      dataHub{},
	"backup config":               map[string]any{},
	"backup create":               dataResult{},
	"backup drill":                backupDrillData{},
	"backup init-key":             dataResult{},
	"backup list":                 []backup.BackupEntry{},
	"backup pitr":                 dataHub{},
	"backup pitr base-backup":     dataResult{},
	"backup pitr disable":         dataResult{},
	"backup pitr enable":          dataResult{},
	"backup pitr restore":         dataResult{},
	"backup pitr status":          backupPITRStatusData{},
	"backup prune":                backup.PruneJSONReport{},
	"backup restore":              dataResult{},
	"backup restore-remote":       dataResult{},
	"backup resume":               dataResult{},
	"backup schedule":             dataResult{},
	"backup status":               backupStatusData{},
	"backup verify":               backup.VerifyResult{},
	"db":                          dataHub{},
	"db backup":                   dataResult{},
	"db backup list":              []backupEntry{},
	"db backup-sync":              dataResult{},
	"db backup-sync-status":       dbSyncStatusData{},
	"db drift":                    dataResult{},
	"db drift fix":                dataResult{},
	"db drift scan":               dbDriftScanData{},
	"db drop":                     dataResult{},
	"db fk-index":                 dataHub{},
	"db fk-index apply":           dataResult{},
	"db fk-index audit":           dbFKIndexAuditData{},
	"db generate":                 dataResult{},
	"db hasura":                   dataHub{},
	"db hasura apply-ref":         dataResult{},
	"db hasura console":           dbConsoleData{},
	"db hasura diff":              dataResult{},
	"db hasura git-status":        dbGitStatusData{},
	"db hasura metadata":          dataHub{},
	"db hasura metadata apply":    dataResult{},
	"db hasura metadata export":   dataResult{},
	"db hasura metadata reload":   dataResult{},
	"db hasura metadata-drift":    dataResult{},
	"db hasura snapshot":          dataResult{},
	"db hasura sync":              dataResult{},
	"db hasura validate":          dataResult{},
	"db import firebase":          dataResult{},
	"db import supabase":          dataResult{},
	"db lint":                     dbLintData{},
	"db list":                     dataResult{},
	"db migrate":                  dataHub{},
	"db migrate apply":            dataResult{},
	"db migrate audit":            dbMigrateAuditData{},
	"db migrate baseline":         dataResult{},
	"db migrate create":           dataResult{},
	"db migrate down":             dataResult{},
	"db migrate generate":         dataResult{},
	"db migrate idempotent":       dataResult{},
	"db migrate lint":             dbMigrateLintData{},
	"db migrate status":           dbMigrateStatusData{},
	"db migrate up":               dataResult{},
	"db pgbouncer":                dataHub{},
	"db pgbouncer connection-url": dataResult{},
	"db pgbouncer generate":       dataResult{},
	"db pgbouncer status":         dbPgBouncerStatusData{},
	"db pitr":                     dataHub{},
	"db pitr enable":              dataResult{},
	"db pitr restore":             dataResult{},
	"db pitr status":              dbPITRStatusData{},
	"db pitr test":                dataResult{},
	"db reconcile":                dbReconcileData{},
	"db reset":                    dataResult{},
	"db reset-checksum":           dataResult{},
	"db restore":                  dataResult{},
	"db restore-drill":            dataResult{},
	"db restore-drill-list":       dbRestoreDrillListData{},
	"db rls":                      dataHub{},
	"db rls apply":                dataResult{},
	"db rls apply-table":          dataResult{},
	"db rls audit":                []database.RLSTableInfo{},
	"db rls rollback":             dataResult{},
	"db seed":                     pluginSeedData{},
	"db seed fixtures":            dataHub{},
	"db seed fixtures list":       dataResult{},
	"db seed fixtures manifest":   dataResult{},
	"db seed fixtures run":        dataResult{},
	"db seed fixtures verify":     dataResult{},
	"db seed graph":               dbSeedGraphData{},
	"db seed list":                dbSeedListData{},
	"db seed matrix":              dataResult{},
	"db seed run":                 dataResult{},
	"db seed verify":              dbSeedVerifyData{},
	"db soft-delete":              dataHub{},
	"db soft-delete apply":        dataResult{},
	"db soft-delete audit":        dbSoftDeleteAuditData{},
	"db soft-delete generate":     dataResult{},
	"db verify":                   dbVerifyData{},
	"db verify-checksums":         dataResult{},
	"init template":               dataHub{},
	"init template info":          templateEntry{},
	"init template list":          templateListData{},
	"init template publish":       dataResult{},
	"init template update":        dataResult{},
	"update project detect":       dataResult{},
	"update project from-bash":    dataResult{},
	"update project from-v099":    dataResult{},
	"update project rollback":     dataResult{},
	"update project run":          dataResult{},
}

func init() {
	for path, zero := range dataEnvelopeTypes {
		registerJSONType(path, zero)
		registerV15OnlyEnvelope(path)
	}
}
