package commands

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/postgres/imageswitch"
)

func TestImageSwitchConfigPreservesAbsentEnvKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := imageSwitchConfig(dir, &config.Config{Postgres: config.PostgresConfig{Image: "inherited:16", DataVolume: "inherited_data"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{imageswitch.EnvImage, imageswitch.EnvDataVolume} {
		if _, present := cfg.PrevEnv[key]; present {
			t.Errorf("absent %s was captured as an assignment; rollback must remove it", key)
		}
	}
}

func TestImageSwitchEnvApplierRestoresPresence(t *testing.T) {
	for _, previous := range []string{"", "POSTGRES_IMAGE=\"\"\n", "POSTGRES_IMAGE=postgres:16-alpine\nPOSTGRES_DATA_VOLUME=original_data\n"} {
		t.Run(previous, func(t *testing.T) {
			restoreProcessEnv(t)
			for _, key := range []string{imageswitch.EnvImage, imageswitch.EnvDataVolume} {
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			dir := makeNSelfProject(t, "PROJECT_NAME=p\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\nMINIO_ENABLED=false\n"+previous)
			cfg, err := imageSwitchConfig(dir, &config.Config{})
			if err != nil {
				t.Fatal(err)
			}
			ap := &envApplier{dir: dir, stderr: io.Discard}
			if err := ap.Apply(context.Background(), cfg.PrevEnv); err != nil {
				t.Fatal(err)
			}
			baselineEnv, err := os.ReadFile(filepath.Join(dir, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			baseline, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
			if err != nil {
				t.Fatal(err)
			}
			if err := ap.Apply(context.Background(), map[string]string{imageswitch.EnvImage: "pgvector/pgvector:pg16", imageswitch.EnvDataVolume: "new_data"}); err != nil {
				t.Fatal(err)
			}
			// A separate rollback invocation first loads the switched config.
			if _, err := config.Load(dir); err != nil {
				t.Fatal(err)
			}
			if err := ap.Apply(context.Background(), cfg.PrevEnv); err != nil {
				t.Fatal(err)
			}
			restored, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
			if err != nil || !bytes.Equal(restored, baseline) {
				t.Fatalf("rollback did not regenerate the original compose: %v", err)
			}
			restoredEnv, err := os.ReadFile(filepath.Join(dir, ".env"))
			if err != nil || !bytes.Equal(restoredEnv, baselineEnv) {
				t.Fatalf("rollback did not restore the original env bytes: %v", err)
			}
			got, err := config.ReadEnvFile(filepath.Join(dir, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{imageswitch.EnvImage, imageswitch.EnvDataVolume} {
				value, present := got[key]
				want, had := cfg.PrevEnv[key]
				if value != want || present != had {
					t.Errorf("%s = %q present=%v, want %q present=%v", key, value, present, want, had)
				}
			}
		})
	}
}
