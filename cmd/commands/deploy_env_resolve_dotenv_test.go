package commands

// deploy_env_resolve_dotenv_test.go — D-0444/D-0558 for the deploy env
// snapshot: readEnvFileOverrides and writeResolvedDeployEnv report a malformed
// cascade file by name and line, never by its text.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEnvFileOverridesDotenvSyntaxError(t *testing.T) {
	for _, tc := range dotenvLeakCases {
		t.Run(tc.name, func(t *testing.T) {
			path := dotenvPlantFile(t, tc.content)
			_, err := readEnvFileOverrides(path)
			if err == nil {
				t.Fatal("want a syntax error")
			}
			assertDotenvNoLeak(t, err.Error(), path, 2)
		})
	}
	// Missing stays "no overrides, no error".
	if kv, err := readEnvFileOverrides(filepath.Join(t.TempDir(), "absent.env")); kv != nil || err != nil {
		t.Errorf("missing file: %v %v", kv, err)
	}
}

func TestWriteResolvedDeployEnvDotenvSyntaxError(t *testing.T) {
	for _, tc := range dotenvLeakCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			dotenvPlantFile(t, tc.content) // positive control
			_, cleanup, err := writeResolvedDeployEnv(dir, "local")
			defer cleanup()
			if err == nil {
				t.Fatal("want a syntax error")
			}
			assertDotenvNoLeak(t, err.Error(), filepath.Join(dir, ".env"), 2)
			if strings.Count(err.Error(), filepath.Join(dir, ".env")) != 1 {
				t.Errorf("file named more than once: %q", err.Error())
			}
		})
	}
}
