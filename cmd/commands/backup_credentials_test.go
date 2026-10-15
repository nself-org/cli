package commands

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBackupScheduleProdMinioCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("systemd scheduling requires POSIX paths")
	}
	prodBackupMinioProject(t)
	if err := os.WriteFile("nself.yaml", []byte("name: backup-prod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := backupScheduleCmd.Flags()
	resetBackupFlagSet(t, fs)
	for name, value := range map[string]string{"to": "r2:bucket/backups", "cron": "30 2 * * *", "dry-run": "true", "recipient": "age1test"} {
		if err := fs.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	backupScheduleCmd.SetOut(io.Discard)
	t.Cleanup(func() { backupScheduleCmd.SetOut(nil) })
	if err := runBackupSchedule(backupScheduleCmd, nil); err != nil {
		t.Fatalf("stream scheduling rejected unused MinIO credentials: %v", err)
	}
}

// TestBackupStreamProdMinioCredentials exercises the command's real loader:
// database-only streams must not require credentials for the MinIO service.
func TestBackupStreamProdMinioCredentials(t *testing.T) {
	prodBackupMinioProject(t)
	fs := backupStreamCmd.Flags()
	resetBackupFlagSet(t, fs)
	for name, value := range map[string]string{"to": "r2:bucket/backups", "dry-run": "true", "no-encrypt": "true"} {
		if err := fs.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := runBackupStream(backupStreamCmd, nil); err != nil {
		t.Fatalf("database-only stream rejected unused MinIO credentials: %v", err)
	}
	// Service-aware callers must continue to reject the same credentials.
	if _, err := loadProjectConfig(); err == nil || !strings.Contains(err.Error(), "MINIO_ROOT_USER") {
		t.Fatalf("normal project load must retain MinIO guard, got %v", err)
	}
}

func prodBackupMinioProject(t *testing.T) {
	t.Helper()
	for _, key := range []string{"ENV", "MINIO_ENABLED", "STORAGE_ENABLED", "MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD", "NSELF_V15", "IMAGE_PINNING"} {
		t.Setenv(key, "")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("ENV=prod\nPROJECT_NAME=backup-prod\nBASE_DOMAIN=example.com\nMINIO_ENABLED=true\nMINIO_ROOT_USER=minioadmin\nMINIO_ROOT_PASSWORD=minioadmin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}
