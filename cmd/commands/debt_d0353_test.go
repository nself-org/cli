package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/secrets"
)

func TestD0353JSONKeyRotation(t *testing.T) {
	requireAgeLoud(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)
		secretsEnvFlag = "dev"
		_ = secretsRotateCmd.Flags().Set("dual-window", "false")
		for _, algorithm := range []string{"HS256", "HS384", "HS512"} {
			old := `{"type":"` + algorithm + `","key":"old-key","claims_namespace":"custom","audience":["one"],"custom":9007199254740993}`
			if err := secrets.Set(root, "dev", "HASURA_GRAPHQL_JWT_SECRET", old); err != nil {
				t.Fatal(err)
			}
			if err := secretsRotateCmd.RunE(secretsRotateCmd, []string{"HASURA_GRAPHQL_JWT_SECRET"}); err != nil {
				t.Fatal(err)
			}
			got, err := secrets.Get(root, "dev", "HASURA_GRAPHQL_JWT_SECRET")
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(got), &fields); err != nil {
				t.Fatalf("rotation replaced JWT JSON with a scalar: %v", err)
			}
			var before map[string]json.RawMessage
			_ = json.Unmarshal([]byte(old), &before)
			for name, value := range before {
				if name != "key" && string(fields[name]) != string(value) {
					t.Errorf("rotation changed field %s", name)
				}
			}
			var key string
			if err := json.Unmarshal(fields["key"], &key); err != nil || key == "old-key" || len(key) < 64 {
				t.Fatalf("JSON key did not rotate safely")
			}
		}
	})
}

func TestD0353ComposeCascadeDoesNotChangeHash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub docker is a POSIX shell script")
	}
	for _, operation := range []string{"restart", "up"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("ENV", "dev")
			t.Setenv("D0353_VALUE", "caller-conflict")
			t.Setenv("PROJECT_NAME", "")
			t.Setenv("COMPOSE_PROJECT_NAME", "unexpected-shell-project")
			for name, content := range map[string]string{
				".env":               "PROJECT_NAME=d0353\nD0353_VALUE=base\n",
				".env.dev":           "D0353_VALUE=dev-conflict\n",
				".nself/compose.env": "D0353_VALUE=running-definition\n",
			} {
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if os.Getenv("D0353_VALUE") != "dev-conflict" {
				t.Fatal("fixture did not load the conflicting .env.dev value")
			}
			output := filepath.Join(dir, "rendered-hash")
			fake := filepath.Join(dir, "docker")
			// Model Compose interpolation precedence and hash the rendered
			// definition: process values win over the last --env-file value.
			script := "#!/bin/sh\nfile_value=\nwhile [ $# -gt 0 ]; do\n" +
				"if [ \"$1\" = --env-file ]; then shift; file_value=$(sed -n 's/^D0353_VALUE=//p' \"$1\"); fi\nshift\ndone\n" +
				"printf '%s' \"${D0353_VALUE-$file_value}\" | cksum > '" + output + "'\n"
			if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			comp := docker.NewCompose()
			comp.DockerPath = fake
			comp.EnvFiles = build.ComposeEnvFiles(dir)
			// Baseline: the hash of the deployed env-file definition.
			_ = os.Unsetenv("D0353_VALUE")
			if err := comp.ComposeUp(context.Background(), dir); err != nil {
				t.Fatal(err)
			}
			baseline, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			_ = os.Setenv("D0353_VALUE", "dev-conflict")
			if operation == "restart" {
				err = restartServices(context.Background(), comp, dir, cfg, []string{"hasura"}, false)
			} else {
				err = startRemainingServicesPhase(context.Background(), comp, dir, cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(got)) != strings.TrimSpace(string(baseline)) {
				t.Fatalf("conflicting .env.dev changed rendered definition hash: got %s, want %s", got, baseline)
			}
			if os.Getenv("D0353_VALUE") != "dev-conflict" {
				t.Fatal("compose invocation did not restore caller environment")
			}
		})
	}
}

func TestD0353JSONRotationRejectsUnsafeShapes(t *testing.T) {
	requireAgeLoud(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)
		secretsEnvFlag = "dev"
		for _, value := range []string{
			`{"type":"HS256","audience":"fixture","key":`,
			`{"type":"HS256","audience":"fixture"}`,
			`{"type":"HS256","audience":"fixture","key":42}`,
			`{"type":"HS256","audience":"fixture","key":""}`,
			`{"type":"RS256","key":"public-key"}`,
			`{"jwk_url":"https://example.invalid/jwks"}`,
		} {
			if err := secrets.Set(root, "dev", "HASURA_GRAPHQL_JWT_SECRET", value); err != nil {
				t.Fatal(err)
			}
			_ = secretsRotateCmd.Flags().Set("dual-window", "false")
			if err := secretsRotateCmd.RunE(secretsRotateCmd, []string{"HASURA_GRAPHQL_JWT_SECRET"}); err == nil {
				t.Fatal("unsafe JWT configuration was rotated")
			}
			got, err := secrets.Get(root, "dev", "HASURA_GRAPHQL_JWT_SECRET")
			if err != nil || got != value {
				t.Fatal("failed rotation clobbered the original JSON value")
			}
		}
		value := `{"type":"HS256","key":"old-key"}`
		if err := secrets.Set(root, "dev", "HASURA_GRAPHQL_JWT_SECRET", value); err != nil {
			t.Fatal(err)
		}
		_ = secretsRotateCmd.Flags().Set("dual-window", "true")
		defer func() { _ = secretsRotateCmd.Flags().Set("dual-window", "false") }()
		if err := secretsRotateCmd.RunE(secretsRotateCmd, []string{"HASURA_GRAPHQL_JWT_SECRET"}); err == nil {
			t.Fatal("JSON rotation silently used scalar dual-window rotation")
		}
		got, err := secrets.Get(root, "dev", "HASURA_GRAPHQL_JWT_SECRET")
		if err != nil || got != value {
			t.Fatal("refused dual-window rotation modified the original")
		}
		if _, err := secrets.Get(root, "dev", "HASURA_GRAPHQL_JWT_SECRET_PREVIOUS"); err == nil {
			t.Fatal("refused dual-window rotation created a previous key")
		}
	})
}
