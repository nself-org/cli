package backup

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestRequireCompleteS3Credentials_BothEmptyIsFine covers the common case:
// no BACKUP_S3_* / BACKUP_*_KEY vars set at all, remote upload relies
// entirely on rclone.conf or RCLONE_CONFIG_* env vars. This must not be
// treated as "half-configured".
func TestRequireCompleteS3Credentials_BothEmptyIsFine(t *testing.T) {
	cfg := &config.Config{}
	if err := requireCompleteS3Credentials(cfg); err != nil {
		t.Errorf("want nil for both-empty (rclone.conf-only) config, got %v", err)
	}
}

// TestRequireCompleteS3Credentials_BothSetIsFine covers full nSelf-side
// configuration via either the canonical or alias var names — both should
// already have landed in the same struct fields by the time this runs.
func TestRequireCompleteS3Credentials_BothSetIsFine(t *testing.T) {
	cfg := &config.Config{Backup: config.BackupConfig{
		S3AccessKeyID:     "AKIAEXAMPLE",
		S3SecretAccessKey: "supersecret",
	}}
	if err := requireCompleteS3Credentials(cfg); err != nil {
		t.Errorf("want nil for fully-configured S3 credentials, got %v", err)
	}
}

// TestRequireCompleteS3Credentials_HalfConfiguredFails is the regression
// lock for the ntask defect: BACKUP_ACCESS_KEY set without
// BACKUP_SECRET_KEY (or the canonical pair split the same way) must fail
// loudly rather than silently attempt an upload with an empty secret.
func TestRequireCompleteS3Credentials_HalfConfiguredFails(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.BackupConfig
	}{
		{"access only", config.BackupConfig{S3AccessKeyID: "AKIAEXAMPLE"}},
		{"secret only", config.BackupConfig{S3SecretAccessKey: "supersecret"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Backup: tc.cfg}
			if err := requireCompleteS3Credentials(cfg); err == nil {
				t.Errorf("want error for half-configured S3 credentials (%s), got nil", tc.name)
			}
		})
	}
}

// TestRequireBackupContainerConfig_MissingVarsFailLoud is the regression
// lock for "nself backup create failed twice with a confusing error when
// PROJECT_NAME/POSTGRES_DB were unset" — the guard must name the missing
// variable(s) instead of letting a placeholder default silently derive a
// container name that cannot exist.
func TestRequireBackupContainerConfig_MissingVarsFailLoud(t *testing.T) {
	t.Setenv("PROJECT_NAME", "")
	t.Setenv("POSTGRES_DB", "")
	err := requireBackupContainerConfig()
	if err == nil {
		t.Fatal("want error when PROJECT_NAME and POSTGRES_DB are both unset, got nil")
	}
	if !strings.Contains(err.Error(), "PROJECT_NAME") || !strings.Contains(err.Error(), "POSTGRES_DB") {
		t.Errorf("error must name both missing vars, got: %v", err)
	}
}

// TestRequireBackupContainerConfig_SetVarsPass proves the guard is silent
// once both vars are actually configured.
func TestRequireBackupContainerConfig_SetVarsPass(t *testing.T) {
	t.Setenv("PROJECT_NAME", "ntask")
	t.Setenv("POSTGRES_DB", "ntask")
	if err := requireBackupContainerConfig(); err != nil {
		t.Errorf("want nil once PROJECT_NAME/POSTGRES_DB are set, got %v", err)
	}
}
