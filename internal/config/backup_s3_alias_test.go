package config

// Purpose: Regression tests for the BACKUP_ACCESS_KEY/BACKUP_SECRET_KEY alias
//          onto BACKUP_S3_ACCESS_KEY_ID/BACKUP_S3_SECRET_ACCESS_KEY.
//
// Background: ntask/backend/.env.example configures remote backup upload
// with BACKUP_ACCESS_KEY / BACKUP_SECRET_KEY (alongside BACKUP_S3_ENDPOINT),
// but loader_parse_env_ops.go only ever read the longer
// BACKUP_S3_ACCESS_KEY_ID / BACKUP_S3_SECRET_ACCESS_KEY names. The shorter
// names were on loader_known_vars_ops.go's list (so no "unknown env var"
// warning fired) but never reached cfg.Backup.S3AccessKeyID/S3SecretAccessKey
// — remote upload was silently non-functional. Mirrors the same shape as
// minio_alias_test.go's MINIO_ACCESS_KEY/MINIO_SECRET_KEY fix.
//
// Inputs:  Simulated .env cascade values via t.Setenv.
// Outputs: none (t.Error/t.Fatal on assertion failure).

import "testing"

// TestBackupS3Alias_ShortNamesMapToCanonicalFields verifies BACKUP_ACCESS_KEY
// / BACKUP_SECRET_KEY populate S3AccessKeyID/S3SecretAccessKey when the
// canonical BACKUP_S3_* names are not set.
func TestBackupS3Alias_ShortNamesMapToCanonicalFields(t *testing.T) {
	t.Setenv("BACKUP_S3_ACCESS_KEY_ID", "")
	t.Setenv("BACKUP_S3_SECRET_ACCESS_KEY", "")
	t.Setenv("BACKUP_ACCESS_KEY", "r2-access-key")
	t.Setenv("BACKUP_SECRET_KEY", "r2-secret-key")

	cfg := parseEnvToConfig()
	if cfg.Backup.S3AccessKeyID != "r2-access-key" {
		t.Errorf("Backup.S3AccessKeyID = %q, want %q (aliased from BACKUP_ACCESS_KEY)", cfg.Backup.S3AccessKeyID, "r2-access-key")
	}
	if cfg.Backup.S3SecretAccessKey != "r2-secret-key" {
		t.Errorf("Backup.S3SecretAccessKey = %q, want %q (aliased from BACKUP_SECRET_KEY)", cfg.Backup.S3SecretAccessKey, "r2-secret-key")
	}
}

// TestBackupS3Alias_CanonicalNamesWin verifies BACKUP_S3_ACCESS_KEY_ID /
// BACKUP_S3_SECRET_ACCESS_KEY take priority when both the canonical and
// alias vars are set.
func TestBackupS3Alias_CanonicalNamesWin(t *testing.T) {
	t.Setenv("BACKUP_S3_ACCESS_KEY_ID", "canonical-access")
	t.Setenv("BACKUP_S3_SECRET_ACCESS_KEY", "canonical-secret")
	t.Setenv("BACKUP_ACCESS_KEY", "alias-access")
	t.Setenv("BACKUP_SECRET_KEY", "alias-secret")

	cfg := parseEnvToConfig()
	if cfg.Backup.S3AccessKeyID != "canonical-access" {
		t.Errorf("Backup.S3AccessKeyID = %q, want %q (canonical must win)", cfg.Backup.S3AccessKeyID, "canonical-access")
	}
	if cfg.Backup.S3SecretAccessKey != "canonical-secret" {
		t.Errorf("Backup.S3SecretAccessKey = %q, want %q (canonical must win)", cfg.Backup.S3SecretAccessKey, "canonical-secret")
	}
}

// TestBackupS3Alias_NeitherSet_StaysEmpty verifies no credentials invented
// from thin air when nothing at all is set.
func TestBackupS3Alias_NeitherSet_StaysEmpty(t *testing.T) {
	t.Setenv("BACKUP_S3_ACCESS_KEY_ID", "")
	t.Setenv("BACKUP_S3_SECRET_ACCESS_KEY", "")
	t.Setenv("BACKUP_ACCESS_KEY", "")
	t.Setenv("BACKUP_SECRET_KEY", "")

	cfg := parseEnvToConfig()
	if cfg.Backup.S3AccessKeyID != "" {
		t.Errorf("Backup.S3AccessKeyID = %q, want empty", cfg.Backup.S3AccessKeyID)
	}
	if cfg.Backup.S3SecretAccessKey != "" {
		t.Errorf("Backup.S3SecretAccessKey = %q, want empty", cfg.Backup.S3SecretAccessKey)
	}
}
