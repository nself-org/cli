package compose

// Purpose: D-0334 regression: core service DATABASE_URL values reference the
//          password (and its URL-encoded twin) instead of carrying it.
// Inputs:  a config whose Postgres password needs percent-encoding.
// Outputs: custom-service and hasura env, plus the shared helper.
// Constraints: no disk or network.

import (
	"strings"
	"testing"
)

const awkwardPGPassword = "p@ss:w/rd#1234"

func TestDatabaseURLRefHidesAwkwardPassword(t *testing.T) {
	cfg := minimalConfigWithCS()
	cfg.Postgres.Password = awkwardPGPassword

	want := "${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}"
	if got := databaseURLRef(cfg); !strings.Contains(got, ":"+want+"@") {
		t.Fatalf("databaseURLRef = %q, want the %s reference in the credential position", got, want)
	}

	envs := map[string]map[string]string{"custom service": coreEnvVars(cfg, testCS(), nil)}
	hasura, err := NewGenerator(cfg).buildHasuraService()
	if err != nil {
		t.Fatal(err)
	}
	envs["hasura"] = hasura.Environment
	for name, env := range envs {
		for key, val := range env {
			if !strings.HasSuffix(key, "DATABASE_URL") {
				continue
			}
			if strings.Contains(val, awkwardPGPassword) || strings.Contains(val, "p%40ss") {
				t.Errorf("%s %s carries the password: %q", name, key, val)
			}
			if !strings.Contains(val, want) {
				t.Errorf("%s %s = %q, want the %s reference", name, key, val, want)
			}
		}
	}
}
