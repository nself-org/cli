package config

// Purpose: D-0334: TYPESENSE_API_KEY is validated like MEILISEARCH_MASTER_KEY
//          when Typesense is the search engine.
// Inputs:  a config with a strong Postgres password and Hasura secret.
// Outputs: validatePasswords results for empty, short and strong keys.
// Constraints: staging/prod rules (checkPatterns=true); no disk or network.

import (
	"strings"
	"testing"
)

func TestValidatePasswordsTypesenseKey(t *testing.T) {
	base := func() *Config {
		cfg := &Config{}
		cfg.Postgres.Password = "Xk9vQ2mZ7rT4wB8nL5cD"
		cfg.Hasura.AdminSecret = "Hj3pR8sV1xN6yK0qW5eU9tM2bF7gA4dC"
		return cfg
	}
	for _, tc := range []struct {
		name, engine, key string
		enabled           bool
		wantErr           bool
	}{
		{"typesense empty key", "typesense", "", true, true},
		{"typesense short key", "Typesense", "short", true, true},
		{"typesense strong key", "typesense", "Qm4xT9vB2kR7nZ5wL8cH", true, false},
		{"typesense disabled", "typesense", "", false, false},
		{"other engine ignores the key", "meilisearch", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			cfg.Search.Enabled = tc.enabled
			cfg.Search.Engine = tc.engine
			cfg.Search.Typesense.APIKey = tc.key
			cfg.Search.MeiliSearch.MasterKey = "Wd6yN1pK9sF3vC8tG2hJ"
			err := validatePasswords(cfg, true)
			if tc.wantErr != (err != nil) {
				t.Fatalf("validatePasswords = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "TYPESENSE_API_KEY") {
				t.Fatalf("error %q does not name TYPESENSE_API_KEY", err)
			}
		})
	}
}
