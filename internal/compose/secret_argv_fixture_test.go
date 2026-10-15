package compose_test

import (
	"os"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

// argvSecretsConfig supplies distinct synthetic values for every mapped secret.
func argvSecretsConfig(engine string) *config.Config {
	cfg := optionalSecretsConfig(engine, setSecrets)
	cfg.Minio.S3AccessKey = "s3AccessSentinel0123456789"
	cfg.Minio.S3SecretKey = "s3SecretSentinel0123456789"
	cfg.Auth.SMTPPass = "smtpSentinel0123456789"
	cfg.Monitoring.GrafanaAdminPassword = "grafanaSentinel0123456789"
	cfg.Search.APIKey = "searchSentinel0123456789"
	cfg.PluginSystem.InternalSecret = "pluginSentinel0123456789"
	cfg.PluginConfig.NotifySecret = "notifySentinel0123456789"
	return cfg
}

// TestArgvFixtureLeakControls checks every secret against each parsed argv
// field, including the plugin keys and scalar/list YAML representations.
func TestArgvFixtureLeakControls(t *testing.T) {
	secrets := build.SecretEnvMap(argvSecretsConfig("meilisearch"))
	for key, value := range secrets {
		for _, field := range []string{"command", "entrypoint", "healthcheck.test"} {
			for _, leak := range []string{value, "${" + key + "}", "$" + key} {
				svc := map[string]any{field: leak}
				if field == "healthcheck.test" {
					svc = map[string]any{"healthcheck": map[string]any{"test": []string{"CMD", leak}}}
				}
				data, err := yaml.Marshal(map[string]any{"services": map[string]any{"probe": svc}})
				if err != nil {
					t.Fatal(err)
				}
				if got := argvSecretFindings(argvStrings(t, data), secrets); len(got) == 0 {
					t.Errorf("guard missed %s in %s", key, field)
				}
			}
			if got := argvSecretFindings(map[string][]string{field: {"$$" + key}}, secrets); len(got) != 0 {
				t.Errorf("escaped %s in %s flagged: %v", key, field, got)
			}
		}
	}
}

// TestNoSecretOnPluginArgv applies the generated-argv guard to a representative
// plugin fragment, which is a separate compose file in the built stack.
func TestNoSecretOnPluginArgv(t *testing.T) {
	data, err := os.ReadFile("testdata/secret-argv-plugin.yml")
	if err != nil {
		t.Fatal(err)
	}
	secrets := build.SecretEnvMap(argvSecretsConfig("meilisearch"))
	argv := argvStrings(t, build.TemplateSecrets(data, secrets))
	for _, field := range []string{"command", "entrypoint", "healthcheck.test"} {
		if len(argv["notify "+field]) == 0 {
			t.Fatalf("plugin %s missing; the guard would not cover it", field)
		}
	}
	for _, finding := range argvSecretFindings(argv, secrets) {
		t.Error(finding)
	}
}

// TestArgvFixtureCoversSecrets prevents empty fixture values from silently
// excluding secret references from the generated-argv guard.
func TestArgvFixtureCoversSecrets(t *testing.T) {
	cfg := argvSecretsConfig("meilisearch")
	secrets := build.SecretEnvMap(cfg)
	for _, key := range []string{
		"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", "HASURA_JWT_KEY",
		"HASURA_GRAPHQL_JWT_SECRET", "MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD",
		"S3_ACCESS_KEY", "S3_SECRET_KEY", "REDIS_PASSWORD", "AUTH_SMTP_PASS",
		"MAILPIT_UI_PASSWORD", "ADMIN_SECRET_KEY", "ADMIN_PASSWORD_HASH",
		"GRAFANA_ADMIN_PASSWORD", "SEARCH_API_KEY", "MEILISEARCH_MASTER_KEY",
		"TYPESENSE_API_KEY", "PLUGIN_INTERNAL_SECRET", "NOTIFY_INTERNAL_SECRET",
	} {
		if len(secrets[key]) < 4 {
			t.Errorf("argv fixture does not cover %s with a synthetic secret", key)
		}
	}
}
