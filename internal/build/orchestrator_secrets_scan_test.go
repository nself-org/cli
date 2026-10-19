package build

// orchestrator_secrets_scan_test.go — the env-file scan reads keys the way the
// loader does (D-1150): `export KEY=v`, `KEY =v` and `KEY: v` all define KEY, so
// a dev build never persists a generated value over a production password.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanEnvFiles_LoaderGrammar(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool // POSTGRES_PASSWORD seen on disk
	}{
		{"plain", "POSTGRES_PASSWORD=prod-secret\n", true},
		{"export prefix", "export POSTGRES_PASSWORD=prod-secret\n", true},
		{"space before equals", "POSTGRES_PASSWORD =prod-secret\n", true},
		{"colon form", "POSTGRES_PASSWORD: prod-secret\n", true},
		{"quoted export", "export POSTGRES_PASSWORD=\"prod secret\"\n", true},
		{"commented", "# POSTGRES_PASSWORD=prod-secret\n", false},
		{"other key", "POSTGRES_PASSWORD_FILE=/run/x\n", false},
		{"unparseable file still names key", "export POSTGRES_PASSWORD=ok\n$bad line\n", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			testWriteFile(t, filepath.Join(dir, ".env.prod"), tc.content, 0600)
			got := scanEnvFiles(dir, "dev", newDiskSink(dir))
			if got["POSTGRES_PASSWORD"] != tc.want {
				t.Fatalf("POSTGRES_PASSWORD on disk = %v, want %v (keys: %v)", got["POSTGRES_PASSWORD"], tc.want, got)
			}
			if got["POSTGRES_PASSWORD "] || got["export POSTGRES_PASSWORD"] {
				t.Fatalf("raw key spelling leaked into the set: %v", got)
			}
		})
	}
}

func TestPersistGeneratedSecrets_DoesNotShadowExportedProdPassword(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "")
	t.Setenv("HASURA_GRAPHQL_ADMIN_SECRET", "")
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env.prod"), "export POSTGRES_PASSWORD=prod-secret\n", 0600)
	cfg := cfgWithJWT()
	cfg.Postgres.Password = "generated-dev-password"
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatalf("persistGeneratedSecrets: %v", err)
	}
	got := scanEnvFiles(dir, "dev", newDiskSink(dir))
	if !got["POSTGRES_PASSWORD"] {
		t.Fatal("key vanished from the scan")
	}
	data := readFileOrEmpty(t, filepath.Join(dir, ".env.secrets"))
	if containsStr(data, "POSTGRES_PASSWORD=") {
		t.Fatalf(".env.secrets now carries POSTGRES_PASSWORD and would override .env.prod:\n%s", data)
	}
}

func readFileOrEmpty(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }
