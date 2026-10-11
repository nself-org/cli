package reconcile

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

// composeFixture builds a config whose redis password is pw and whose other
// secrets are plain synthetic values. Mailpit is off: its UI basic-auth value
// is not covered by the secret-name rule (filed as debt).
func composeFixture(pw string) *config.Config {
	return &config.Config{
		ProjectName:   "test",
		BaseDomain:    "localhost",
		Env:           "staging",
		DockerNetwork: "test_network",
		Postgres:      config.PostgresConfig{Host: "postgres", User: "postgres", Password: "fake-pg-0000", DB: "nself", Port: 5432},
		Hasura: config.HasuraConfig{
			AdminSecret: "fake-hasura-0000", JWTKey: "fake-jwt-key-00000000000000000000",
			JWTType: "HS256", Port: 8080, Version: "v2.36.0", MemLimit: "1g", CPULimit: "1.0",
		},
		Auth:  config.AuthConfig{Version: "0.36.0", Port: 4000, ClientURL: "http://localhost:3000", MemLimit: "256m", CPULimit: "0.25"},
		Redis: config.RedisConfig{Enabled: true, Password: pw},
		Minio: config.MinioConfig{Enabled: true, RootUser: "minio", RootPassword: "fake-minio-0000"},
		Search: config.SearchConfig{
			Enabled: true, Engine: "typesense",
			Typesense: config.TypesenseConfig{APIKey: "fake-typesense-0000"},
		},
	}
}

// legacyRedisCompose is the redis service as generators before cli#666 wrote it:
// a password that is not dotenv-safe went literally into the command and the
// healthcheck. Such files stay on disk until the next build, so the upgrade diff
// still reads them.
func legacyRedisCompose(t *testing.T, pw string) []byte {
	t.Helper()
	out, err := yaml.Marshal(map[string]any{"services": map[string]any{"redis": map[string]any{
		"image":   "redis:7-alpine",
		"command": "redis-server --appendonly yes --protected-mode yes --requirepass " + pw + " --maxclients 50",
		"healthcheck": map[string]any{
			"test": []string{"CMD", "redis-cli", "-a", pw, "ping"},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestUnifiedHidesGeneratedRedisPassword diffs a legacy compose file that holds
// redis passwords which are not dotenv-safe (literal in the command and the
// healthcheck) against itself changed and against the current generator's
// output, and asserts no diff shows Hunter2Leak.
func TestUnifiedHidesGeneratedRedisPassword(t *testing.T) {
	for _, pw := range []string{"pa ss Hunter2Leak", "Hunter2Leak$x", "Hunter2'Leak", `Hunter2"Leak`, `Hunter2\Leak`} {
		t.Run(pw, func(t *testing.T) {
			legacy := legacyRedisCompose(t, pw)
			if !strings.Contains(string(legacy), "Hunter2") {
				t.Fatalf("positive control: legacy compose lacks the password:\n%s", legacy)
			}
			current, err := compose.NewGenerator(composeFixture(pw)).Generate()
			if err != nil {
				t.Fatal(err)
			}
			text := string(legacy)
			changed := strings.Replace(text, "image: redis", "image: redis-x", 1)
			if changed == text {
				t.Fatal("fixture: no redis image line to change")
			}
			for name, diff := range map[string]string{
				"new file": Unified("docker-compose.yml", nil, legacy),
				"change":   Unified("docker-compose.yml", legacy, []byte(changed)),
				"upgrade":  Unified("docker-compose.yml", legacy, current),
				"current":  Unified("docker-compose.yml", nil, current),
			} {
				if diff == "" {
					t.Fatalf("%s: empty diff", name)
				}
				for _, leak := range []string{"Hunter2", "pa ss", "Leak"} {
					if strings.Contains(diff, leak) {
						t.Errorf("%s leaks %q:\n%s", name, leak, diff)
					}
				}
				if strings.Contains(diff, "\x00") {
					t.Errorf("%s holds a NUL byte", name)
				}
			}
		})
	}
}

// TestUnifiedShowsPortChanges proves a plain port or bind-address line prints
// as written (observability.Redact would have hidden the change).
func TestUnifiedShowsPortChanges(t *testing.T) {
	before := "    ports:\n      - 127.0.0.1:5432:5432\n      - 192.168.1.10:80:80\n"
	after := strings.ReplaceAll(before, "5432:5432", "6543:5432")
	got := Unified("docker-compose.yml", []byte(before), []byte(after))
	for _, w := range []string{"-      - 127.0.0.1:5432:5432\n", "+      - 127.0.0.1:6543:5432\n", "192.168.1.10:80:80"} {
		if !strings.Contains(got, w) {
			t.Errorf("diff lacks %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, "\x00") || strings.Contains(got, redactedValue) {
		t.Errorf("plain port diff was altered:\n%s", got)
	}
}
