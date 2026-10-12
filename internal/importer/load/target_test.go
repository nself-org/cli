package load

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/nself-org/cli/internal/config"
	"os"
	"testing"
)

func TestSystemTables(t *testing.T) {
	for _, tc := range []struct {
		schema, name string
		store, want  bool
	}{{"public", "np_plugins", false, true}, {"np_app", "notes", false, true}, {"auth", "users", false, true}, {"hdb_catalog", "tables", false, true}, {"pg_catalog", "tables", false, true}, {"information_schema", "tables", false, true}, {"storage", "files", false, false}, {"storage", "files", true, true}, {"app", "notes", true, false}} {
		if got := SystemTable(tc.schema, tc.name, tc.store); got != tc.want {
			t.Fatalf("%+v got %v", tc, got)
		}
	}
}
func TestActiveStoreEndpoint(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	t.Setenv("SEAWEEDFS_PORT", "18333")
	cfg := &config.Config{}
	cfg.Minio.S3AccessKey = "fixture-access-id"
	cfg.Minio.S3SecretKey = fixtureSecret(t)
	store, err := Store(cfg)
	if err != nil || store.Endpoint != "http://127.0.0.1:18333" {
		t.Fatalf("endpoint missing: %v", err)
	}
	t.Setenv("OBJECT_STORAGE_ENABLED", "false")
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	if store, err = Store(cfg); err != nil || store != nil {
		t.Fatal("disabled store should be absent")
	}
}

// fixtureSecret returns FIXTURE_PASSWORD when set, else a random value; tests never carry literal credentials.
func fixtureSecret(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("FIXTURE_PASSWORD"); v != "" {
		return v
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
