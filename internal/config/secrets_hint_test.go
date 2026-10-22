package config

import (
	"strings"
	"testing"
)

func TestPlaceholderHintNamesSecretsFile(t *testing.T) {
	for _, value := range []string{"", "changeme"} {
		t.Run(value, func(t *testing.T) {
			cfg := validPlaceholderTestConfig("prod")
			cfg.Postgres.Password = value
			err := validateNoPlaceholderSecrets(cfg)
			if err == nil || !strings.Contains(err.Error(), ".env.secrets") {
				t.Fatalf("hint must identify winning file: %v", err)
			}
		})
	}
}
