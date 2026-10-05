package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

func streamTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.ProjectName = "testproj"
	cfg.Postgres.DB = "testdb"
	cfg.Postgres.User = "postgres"
	cfg.Postgres.Password = "pw"
	cfg.Postgres.Host = "localhost"
	cfg.Postgres.Port = 5432
	return cfg
}

// Streaming with no recipient used to skip encryption silently, so
// `nself backup stream --to s3:bucket/path` uploaded a plaintext database dump
// to object storage with nothing in the output saying so. It must fail closed.
func TestStream_NoRecipient_FailsClosed(t *testing.T) {
	compattest.Set(t, false) // v1.4 refusal; the v1.5 behaviour is the test below
	_, err := Stream(context.Background(), streamTestConfig(), StreamOptions{
		To:     "s3:bucket/path",
		DryRun: true,
	})
	if err == nil {
		t.Fatal("streaming with no recipient must not succeed silently")
	}
	msg := err.Error()
	for _, want := range []string{"unencrypted", "--recipient", "--no-encrypt"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q so the operator knows how to proceed; got: %s", want, msg)
		}
	}
}

// v1.5 counterpart: no recipient is not a refusal, it selects the auto identity
// (a dry run creates nothing and still reports an encrypted object).
func TestStream_NoRecipient_AutoKeyInV15(t *testing.T) {
	compattest.Set(t, true)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(NoAutoKeyEnv, "")
	res, err := Stream(context.Background(), streamTestConfig(), StreamOptions{To: "s3:bucket/path", DryRun: true})
	if err != nil {
		t.Fatalf("v1.5 must take the auto-key path, got: %v", err)
	}
	if !res.Encrypted || !strings.HasSuffix(res.BackupID, ".age") {
		t.Errorf("auto-key stream must be encrypted: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); err == nil {
		t.Error("a dry run created key material")
	}
}

// The escape hatch must exist and must be explicit, so an operator who really
// wants a plaintext stream can have one, but never by accident.
func TestStream_NoRecipient_AllowedWhenExplicit(t *testing.T) {
	res, err := Stream(context.Background(), streamTestConfig(), StreamOptions{
		To:               "s3:bucket/path",
		DryRun:           true,
		AllowUnencrypted: true,
	})
	if err != nil {
		t.Fatalf("--no-encrypt should permit an unencrypted stream: %v", err)
	}
	if res.Encrypted {
		t.Error("result claims Encrypted with no recipient configured")
	}
	if strings.HasSuffix(res.BackupID, ".age") {
		t.Errorf("unencrypted object must not carry the .age suffix: %s", res.BackupID)
	}
}

// A configured recipient still produces an encrypted stream, and the object key
// still advertises it.
func TestStream_WithRecipient_Encrypts(t *testing.T) {
	res, err := Stream(context.Background(), streamTestConfig(), StreamOptions{
		To:         "s3:bucket/path",
		Recipients: []string{"age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsxxxxxx"},
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("a configured recipient must stream fine: %v", err)
	}
	if !res.Encrypted {
		t.Error("result should report Encrypted with a recipient configured")
	}
	if !strings.HasSuffix(res.BackupID, ".age") {
		t.Errorf("encrypted object should carry the .age suffix: %s", res.BackupID)
	}
}

// The env-configured recipient path must satisfy the check too, or operators
// who configure encryption globally would be pushed toward --no-encrypt.
func TestStream_EnvRecipient_SatisfiesCheck(t *testing.T) {
	cfg := streamTestConfig()
	cfg.Backup.AgeRecipients = "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsxxxxxx"
	res, err := Stream(context.Background(), cfg, StreamOptions{
		To:     "s3:bucket/path",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("a recipient from config must satisfy the check: %v", err)
	}
	if !res.Encrypted {
		t.Error("config-supplied recipient should produce an encrypted stream")
	}
}
