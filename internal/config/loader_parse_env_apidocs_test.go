package config

// Purpose: tests for parseEnvAPIDocs and the API_DOCS_* env path through Load (D-0455).
// Inputs:  t.Setenv values and a temp project dir with a .env file.
// Outputs: none (t.Error on assertion failure).

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var apiDocsVars = []string{
	"API_DOCS_ENABLED", "API_DOCS_PATH", "API_DOCS_TITLE", "API_DOCS_THEME",
	"API_DOCS_AUTH_ENV_VAR", "API_DOCS_GRAPHQL_ENABLED", "API_DOCS_GRAPHQL_ENDPOINT",
}

// clearAPIDocsEnv blanks every API_DOCS_* variable for the test (restored on cleanup).
func clearAPIDocsEnv(t *testing.T) {
	t.Helper()
	for _, k := range apiDocsVars {
		t.Setenv(k, "")
	}
}

// TestParseEnvAPIDocs covers defaults, the off switch and explicit values.
func TestParseEnvAPIDocs(t *testing.T) {
	defaults := ApiDocsConfig{Enabled: true, Path: "/docs", Theme: "default",
		GraphQLEnabled: true, GraphQLEndpoint: "/v1/graphql"}
	disabled := defaults
	disabled.Enabled = false
	explicit := ApiDocsConfig{Enabled: true, Path: "/reference", Title: "Example API", Theme: "moon",
		AuthEnvVar: "EXAMPLE_TOKEN", GraphQLEnabled: false, GraphQLEndpoint: "/graphql"}
	cases := []struct {
		name string
		env  map[string]string
		want ApiDocsConfig
	}{
		{"unset gives defaults", nil, defaults},
		{"enabled false keeps other defaults", map[string]string{"API_DOCS_ENABLED": "false"}, disabled},
		{"explicit values kept", map[string]string{
			"API_DOCS_ENABLED": "true", "API_DOCS_PATH": "/reference", "API_DOCS_TITLE": "Example API",
			"API_DOCS_THEME": "moon", "API_DOCS_AUTH_ENV_VAR": "EXAMPLE_TOKEN",
			"API_DOCS_GRAPHQL_ENABLED": "false", "API_DOCS_GRAPHQL_ENDPOINT": "/graphql"}, explicit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAPIDocsEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			cfg := &Config{}
			parseEnvAPIDocs(cfg)
			got := cfg.ApiDocs
			got.HideEndpoints = nil
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ApiDocs = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestLoadAPIDocsDisabledFromEnvFile proves API_DOCS_ENABLED=false in a project
// .env reaches the loaded config and does not raise an unknown-var warning.
func TestLoadAPIDocsDisabledFromEnvFile(t *testing.T) {
	clearAPIDocsEnv(t)
	t.Setenv("ENV", "dev")
	dir := t.TempDir()
	body := "PROJECT_NAME=example\nBASE_DOMAIN=example.test\nAPI_DOCS_ENABLED=false\nAPI_DOCS_PATH=/reference\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ApiDocs.Enabled {
		t.Error("ApiDocs.Enabled = true, want false for API_DOCS_ENABLED=false")
	}
	if cfg.ApiDocs.Path != "/reference" {
		t.Errorf("ApiDocs.Path = %q, want /reference", cfg.ApiDocs.Path)
	}
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "unknown env var") && strings.Contains(line, "API_DOCS_") {
			t.Errorf("unexpected unknown-var warning: %s", line)
		}
	}
}
