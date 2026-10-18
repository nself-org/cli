package commands

// config_get_cascade_test.go — D-0919: `config get` resolves a key through the
// same env cascade as `nself build`, so a .env.local override is what it
// prints. Values are obviously fake.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cascadeGetProject writes a project whose .env, .env.dev and .env.local set
// the same key, and returns its directory.
func cascadeGetProject(t *testing.T) string {
	t.Helper()
	t.Setenv("ENV", "dev")
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	dir := makeNSelfProject(t, "ENV=dev\nHASURA_JWT_KEY=fake-from-env-d0919\nONLY_BASE=base-d0919\n")
	for name, body := range map[string]string{
		".env.dev":   "HASURA_JWT_KEY=fake-from-dev-d0919\n",
		".env.local": "HASURA_JWT_KEY=fake-from-local-d0919\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func cascadeGet(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs(append([]string{"config", "get"}, args...))
	return captureStdout(t, func() error { return root.Execute() })
}

func TestConfigGetFollowsEnvCascade(t *testing.T) {
	cascadeGetProject(t)

	out, err := cascadeGet(t, "HASURA_JWT_KEY", "--reveal")
	if err != nil {
		t.Fatalf("config get: %v", err)
	}
	if got := strings.TrimSpace(out); got != "fake-from-local-d0919" {
		t.Errorf("config get printed %q, want the .env.local override (what build uses)", got)
	}

	// A key only the base file sets still resolves.
	out, err = cascadeGet(t, "ONLY_BASE")
	if err != nil {
		t.Fatalf("config get ONLY_BASE: %v", err)
	}
	if got := strings.TrimSpace(out); got != "base-d0919" {
		t.Errorf("config get ONLY_BASE printed %q, want base-d0919", got)
	}
}

func TestConfigGetNamesTheWinningFile(t *testing.T) {
	cascadeGetProject(t)
	t.Setenv("NSELF_V15", "1")

	out, err := cascadeGet(t, "HASURA_JWT_KEY", "--reveal", "--json")
	if err != nil {
		t.Fatalf("config get --json: %v", err)
	}
	var env struct {
		Data configGetData `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if env.Data.File != ".env.local" || env.Data.Value != "fake-from-local-d0919" {
		t.Errorf("json data = %+v, want file .env.local and the override value", env.Data)
	}
}

func TestConfigGetExplicitEnvReadsThatFileOnly(t *testing.T) {
	cascadeGetProject(t)

	out, err := cascadeGet(t, "HASURA_JWT_KEY", "--reveal", "--env", "dev")
	if err != nil {
		t.Fatalf("config get --env dev: %v", err)
	}
	if got := strings.TrimSpace(out); got != "fake-from-dev-d0919" {
		t.Errorf("--env dev printed %q, want the .env.dev value", got)
	}
}
