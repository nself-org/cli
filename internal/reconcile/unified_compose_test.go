package reconcile

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
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

// TestUnifiedHidesGeneratedRedisPassword runs the real compose generator with
// redis passwords that are not dotenv-safe (so the generator writes them
// literally into the command and the healthcheck) and asserts the diff never
// shows Hunter2Leak, for a new file and for a before/after pair.
func TestUnifiedHidesGeneratedRedisPassword(t *testing.T) {
	for _, pw := range []string{"pa ss Hunter2Leak", "Hunter2Leak$x", "Hunter2'Leak", `Hunter2"Leak`, `Hunter2\Leak`} {
		t.Run(pw, func(t *testing.T) {
			out, err := compose.NewGenerator(composeFixture(pw)).Generate()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(out), "Hunter2") {
				t.Fatalf("positive control: generated compose lacks the password:\n%s", out)
			}
			text := string(out)
			changed := strings.Replace(text, "image: redis", "image: redis-x", 1)
			if changed == text {
				t.Fatal("fixture: no redis image line to change")
			}
			for name, diff := range map[string]string{
				"new file": Unified("docker-compose.yml", nil, out),
				"change":   Unified("docker-compose.yml", out, []byte(changed)),
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
