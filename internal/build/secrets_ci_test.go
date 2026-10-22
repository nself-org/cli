package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestCIMissingCoreSecretRefusesPersistence(t *testing.T) {
	for _, flag := range []string{"CI", "NSELF_CI"} {
		for _, key := range []string{"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET"} {
			t.Run(flag+"/"+key, func(t *testing.T) {
				clearSecretEnv(t)
				t.Setenv("CI", "")
				t.Setenv("NSELF_CI", "")
				t.Setenv(flag, "1")
				cfg := &config.Config{Env: "dev"}
				if key == "POSTGRES_PASSWORD" {
					cfg.Postgres.Password = "fixture-generated"
				} else {
					cfg.Hasura.AdminSecret = "fixture-generated"
				}
				dir := t.TempDir()
				err := persistGeneratedSecrets(dir, cfg)
				if err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("want missing-key refusal, got %v", err)
				}
				if _, err := os.Stat(filepath.Join(dir, ".env.secrets")); !os.IsNotExist(err) {
					t.Fatal("CI refusal wrote secrets")
				}
			})
		}
	}
}
