package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Purpose: config get must resolve the same cascade as a local build (D-0919).
// Inputs: fake dotenv layers and environment selectors. Outputs: winning value.
// Constraints: no config.Load, process exports, network, or real secrets.
func TestConfigGetCascade(t *testing.T) {
	for _, tc := range []struct {
		name, base, processEnv, flagEnv, legacy, want string
		layers                                        map[string]string
	}{
		{"local override", "HASURA_JWT_KEY=fake-base\n", "", "", "", "fake-local", map[string]string{".env.local": "HASURA_JWT_KEY=fake-local\n"}},
		{"base environment selector", "ENV=prod\nHASURA_JWT_KEY=fake-base\n", "", "", "", "fake-prod", map[string]string{".env.prod": "HASURA_JWT_KEY=fake-prod\n"}},
		{"process environment selector", "ENV=prod\nHASURA_JWT_KEY=fake-base\n", "staging", "", "", "fake-staging", map[string]string{".env.staging": "HASURA_JWT_KEY=fake-staging\n"}},
		{"explicit environment selector", "HASURA_JWT_KEY=fake-base\n", "prod", "qa", "", "fake-qa", map[string]string{".env.qa": "HASURA_JWT_KEY=fake-qa\n"}},
		{"secrets override environment", "HASURA_JWT_KEY=fake-base\n", "", "", "", "fake-secret", map[string]string{".env.dev": "HASURA_JWT_KEY=fake-dev\n", ".env.secrets": "HASURA_JWT_KEY=fake-secret\n"}},
		{"legacy base wins", "HASURA_JWT_KEY=fake-base\n", "", "", "1", "fake-base", map[string]string{".env.local": "HASURA_JWT_KEY=fake-local\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENV", tc.processEnv)
			t.Setenv("NSELF_LEGACY_ENV_ORDER", tc.legacy)
			dir := makeNSelfProject(t, tc.base)
			for name, body := range tc.layers {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			root := newConfigCmd()
			root.SetErr(new(bytes.Buffer))
			root.SetArgs([]string{"config", "get", "HASURA_JWT_KEY", "--reveal", "--env", tc.flagEnv})
			out, err := captureStdout(t, root.Execute)
			if err != nil || out != tc.want+"\n" {
				t.Fatalf("output=%q err=%v, want %q", out, err, tc.want+"\n")
			}
		})
	}
}
