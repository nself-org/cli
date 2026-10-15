package compose

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"gopkg.in/yaml.v3"
)

// D-0551: supported core images receive mounted secrets rather than values
// in Docker Config.Env. Parse generated YAML so serialization is covered too.
func TestGeneratedFileSecrets(t *testing.T) {
	compattest.Set(t, false)
	cfg := minimalConfig()
	cfg.Minio.Enabled = true
	cfg.Minio.RootUser = "minio"
	cfg.Minio.RootPassword = "minio-password"
	raw, err := NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
			Secrets     []string          `yaml:"secrets"`
		} `yaml:"services"`
		Secrets map[string]struct {
			File string `yaml:"file"`
		} `yaml:"secrets"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for service, keys := range map[string][]string{
		"postgres": {"POSTGRES_PASSWORD"},
		"minio":    {"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD"},
		"hasura":   {"HASURA_GRAPHQL_ADMIN_SECRET", "HASURA_GRAPHQL_JWT_SECRET", "HASURA_GRAPHQL_DATABASE_URL"},
	} {
		for _, key := range keys {
			svc := doc.Services[service]
			if _, ok := svc.Environment[key]; ok {
				t.Errorf("%s still exposes %s in Config.Env", service, key)
			}
			if service != "hasura" && svc.Environment[key+"_FILE"] != "/run/secrets/"+strings.ToLower(key) {
				t.Errorf("%s missing file variable for %s", service, key)
			}
			found := false
			for _, grant := range svc.Secrets {
				if grant == strings.ToLower(key) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s missing grant for %s", service, key)
			}
			if doc.Secrets[strings.ToLower(key)].File != "./.nself/secrets/"+key {
				t.Errorf("%s missing file secret source %s", service, key)
			}
		}
	}
}

func TestHasuraFileCommandPreservesBytesAndFailsClosed(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("requires POSIX sh")
	}
	dir := t.TempDir()
	value := "  abc'\"$`\\=\n\n"
	file := filepath.Join(dir, "hasura_graphql_admin_secret")
	if err := os.WriteFile(file, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	script := hasuraFileCommand([]string{"HASURA_GRAPHQL_ADMIN_SECRET"})
	script = strings.ReplaceAll(script, "/run/secrets/", dir+"/")
	script = strings.ReplaceAll(script, `exec "$$HGE_BINARY" serve`, `printf '%s' "$$HASURA_GRAPHQL_ADMIN_SECRET"`)
	script = strings.ReplaceAll(script, "$$", "$")
	got, err := exec.Command("sh", "-ec", script).Output()
	if err != nil || string(got) != value {
		t.Fatalf("runtime reader changed credential bytes: %v", err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("sh", "-ec", script).Run(); err == nil {
		t.Fatal("missing secret must fail before starting Hasura")
	}
}

func TestFileSecretNativeWhitespaceFallback(t *testing.T) {
	cfg := minimalConfig()
	cfg.Postgres.Password = "line\n"
	cfg.Minio.RootUser = " minio "
	cfg.Minio.RootPassword = "password\n"
	dc := &DockerCompose{Services: map[string]ServiceConfig{"postgres": NewGenerator(cfg).buildPostgresService(), "minio": NewGenerator(cfg).buildMinioService()}}
	NewGenerator(cfg).fileSecrets(dc)
	if len(dc.Secrets) != 0 {
		t.Fatal("native readers would strip credential bytes; must retain env")
	}
	if dc.Services["postgres"].Environment["POSTGRES_PASSWORD"] != cfg.Postgres.Password {
		t.Fatal("postgres fallback changed bytes")
	}
	if _, exists := dc.Services["minio"].Environment["MINIO_ROOT_PASSWORD"]; !exists {
		t.Fatal("MinIO whitespace fallback missing")
	}
}

func TestFileSecretVersionsStableAndScoped(t *testing.T) {
	cfg := minimalConfig()
	cfg.Minio.RootUser = "minio"
	cfg.Minio.RootPassword = "password"
	want := SecretFileVersions(cfg)
	for range 20 {
		got := SecretFileVersions(cfg)
		for key, value := range want {
			if got[key] != value {
				t.Fatalf("unstable version %s", key)
			}
		}
	}
	cfg.Hasura.AdminSecret += "rotated"
	got := SecretFileVersions(cfg)
	if got["NSELF_SECRET_VERSION_HASURA"] == want["NSELF_SECRET_VERSION_HASURA"] {
		t.Fatal("Hasura rotation must recreate its mount consumer")
	}
	if got["NSELF_SECRET_VERSION_POSTGRES"] != want["NSELF_SECRET_VERSION_POSTGRES"] || got["NSELF_SECRET_VERSION_MINIO"] != want["NSELF_SECRET_VERSION_MINIO"] {
		t.Fatal("unrelated credentials must not rotate service versions")
	}
}
