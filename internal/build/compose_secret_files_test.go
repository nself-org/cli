package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestComposeSecretFilesPlanAndApply(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Postgres: config.PostgresConfig{Password: "   "}, Hasura: config.HasuraConfig{JWTType: "HS256", JWTKey: "12345678901234567890123456789012"}}
	raw := []byte("secrets:\n  postgres_password:\n    file: ./.nself/secrets/POSTGRES_PASSWORD\n")
	ms := newMemSink(dir, "")
	if _, err := writeComposeSecretFilesVia(ms, dir, cfg, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, composeSecretsDir)); !os.IsNotExist(err) {
		t.Fatal("plan created secret directory")
	}
	exp := ms.snapshot()
	es := newExpectSink(dir, exp)
	if _, err := writeComposeSecretFilesVia(es, dir, cfg, raw); err != nil {
		t.Fatal(err)
	}
	if err := es.commit(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, composeSecretsDir, "POSTGRES_PASSWORD")
	got, err := os.ReadFile(file)
	if err != nil || string(got) != "   " {
		t.Fatal("secret bytes changed")
	}
	info, err := os.Stat(filepath.Dir(file))
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("secret directory must be private")
	}
	info, err = os.Stat(file)
	if err != nil || info.Mode().Perm() != 0444 {
		t.Fatal("non-root container must be able to read secret mount")
	}
	// Rebuilds must tighten an existing directory before replacing read-only files.
	if err := os.Chmod(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := writeComposeSecretFilesVia(newDiskSink(dir), dir, cfg, raw); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(filepath.Dir(file))
	if info.Mode().Perm() != 0700 {
		t.Fatal("rebuild did not restore directory privacy")
	}
}
