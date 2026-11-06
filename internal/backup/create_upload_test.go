package backup

// Purpose: pin C2-02: an encrypted full backup reaches the configured remote,
// and a configured upload that fails or is skipped fails the backup.
// Inputs: fake docker (pg_dump) and age binaries on PATH, path:// remotes.
// Outputs: test assertions only.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// fakeDumpAndAge puts a docker stub that prints a dump and an age stub that
// copies its input to its -o file on PATH.
func fakeDumpAndAge(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stubs are shell scripts")
	}
	bin := t.TempDir()
	stubs := map[string]string{
		"docker": "#!/bin/sh\nprintf 'pg-custom-dump'\n",
		// age -r <recipient> -o <out> <in>
		"age": "#!/bin/sh\ncp \"$5\" \"$4\"\n",
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func encryptedRemoteConfig(remote string) *config.Config {
	cfg := &config.Config{ProjectName: "p"}
	cfg.Postgres.DB = "nself"
	cfg.Backup.Encryption = true
	cfg.Backup.AgeRecipients = "age1testrecipient"
	cfg.Backup.Remote = remote
	return cfg
}

func TestEncryptedCreateUploadsToRemote(t *testing.T) {
	fakeDumpAndAge(t)
	local, dest := t.TempDir(), t.TempDir()
	if err := createFullBackup(t.Context(), encryptedRemoteConfig("path://"+dest), local, "20261105_000000", "", CreateOptions{Type: BackupTypeFull}); err != nil {
		t.Fatalf("create: %v", err)
	}
	want := "p_full_20261105_000000.dump.age"
	if b, err := os.ReadFile(filepath.Join(dest, want)); err != nil || string(b) != "pg-custom-dump" {
		t.Fatalf("remote object %s: %q %v", want, b, err)
	}
	if _, err := os.Stat(filepath.Join(local, want)); err != nil {
		t.Fatalf("local encrypted file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(local, "p_full_20261105_000000.dump")); !os.IsNotExist(err) {
		t.Fatalf("plaintext dump left behind: %v", err)
	}
}

func TestCreateFailsWhenConfiguredUploadFails(t *testing.T) {
	fakeDumpAndAge(t)
	for name, cfg := range map[string]*config.Config{
		// A relative path:// destination is rejected by the destination parser.
		"failing remote": encryptedRemoteConfig("path://relative/dir"),
		"half-set S3 keys": func() *config.Config {
			c := encryptedRemoteConfig("s3://bucket/prefix")
			c.Backup.S3AccessKeyID = "AKIDONLY"
			return c
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			local := t.TempDir()
			err := createFullBackup(t.Context(), cfg, local, "20261105_000001", "", CreateOptions{Type: BackupTypeFull})
			kept := filepath.Join(local, "p_full_20261105_000001.dump.age")
			if err == nil || !strings.Contains(err.Error(), kept) {
				t.Fatalf("want an error naming %s, got %v", kept, err)
			}
			if _, statErr := os.Stat(kept); statErr != nil {
				t.Fatalf("local backup not kept: %v", statErr)
			}
		})
	}
}
