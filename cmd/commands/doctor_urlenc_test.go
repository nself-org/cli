package commands

// Purpose: cmd-level test for the url-reserved-password doctor advisory
// (checkURLReservedPasswords, D-0228). The internal/doctor package tests the
// detector; this test covers the wiring: the compose manifest, which entry is
// the base compose, the result shape, and that no password value is printed.
// Inputs:  synthetic compose fragments and fake passwords in t.TempDir().
// Outputs: test failures only.
// Constraints: fixture passwords are fake strings that hold URL-reserved
//              characters. No real secret, no subprocess, no network.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

const (
	fakePGPassword    = "fake/p@ss:w#rd?%x"
	fakeRedisPassword = "fake+r@w/pw"
	plainPassword     = "fakeplainpassword"
)

// writeURLEncManifest writes a base compose, an optional plugin fragment and
// the manifest listing them by absolute path. An empty fragment writes no
// manifest at all.
func writeURLEncManifest(t *testing.T, fragment string) string {
	t.Helper()
	dir := t.TempDir()
	if fragment == "" {
		return dir
	}
	base := filepath.Join(dir, "docker-compose.yml")
	frag := filepath.Join(dir, "docker-compose.plugin.yml")
	if err := os.WriteFile(base, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(frag, []byte(fragment), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := base + "\n" + frag + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".nself", "compose-files.txt"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckURLReservedPasswords(t *testing.T) {
	cases := []struct {
		name     string
		fragment string
		pgPass   string
		wantVar  string // empty: expect no results
	}{
		{
			name:     "raw postgres password in a URL",
			fragment: "services:\n  svc:\n    environment:\n      DATABASE_URL: postgresql://postgres:${POSTGRES_PASSWORD}@postgres:5432/app\n",
			pgPass:   fakePGPassword,
			wantVar:  "POSTGRES_PASSWORD",
		},
		{
			name:     "safe URLENC form",
			fragment: "services:\n  svc:\n    environment:\n      DATABASE_URL: postgresql://postgres:${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@postgres:5432/app\n",
			pgPass:   fakePGPassword,
		},
		{
			name:     "password without reserved characters",
			fragment: "services:\n  svc:\n    environment:\n      DATABASE_URL: postgresql://postgres:${POSTGRES_PASSWORD}@postgres:5432/app\n",
			pgPass:   plainPassword,
		},
		{
			name:     "raw redis password in a YAML list item",
			fragment: "services:\n  svc:\n    environment:\n      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379\n",
			pgPass:   plainPassword,
			wantVar:  "REDIS_PASSWORD",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeURLEncManifest(t, c.fragment)
			cfg := &config.Config{}
			cfg.Postgres.Password = c.pgPass
			cfg.Redis.Enabled = true
			cfg.Redis.Password = fakeRedisPassword
			got := checkURLReservedPasswords(dir, cfg, false)
			assertNoPasswordLeak(t, got)
			if c.wantVar == "" {
				if len(got) != 0 {
					t.Fatalf("want no results, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want exactly one result, got %+v", got)
			}
			if got[0].Name != "url-reserved-password" || got[0].Status != "warn" {
				t.Errorf("result = %+v, want name url-reserved-password, status warn", got[0])
			}
			if !strings.Contains(got[0].Message, c.wantVar) {
				t.Errorf("message %q does not name %s", got[0].Message, c.wantVar)
			}
		})
	}
}

func TestCheckURLReservedPasswordsNoManifest(t *testing.T) {
	// No manifest: ReadComposeManifest returns only the base compose, so there
	// is no installed fragment to inspect.
	cfg := &config.Config{}
	cfg.Postgres.Password = fakePGPassword
	if got := checkURLReservedPasswords(t.TempDir(), cfg, false); got != nil {
		t.Fatalf("want nil without a manifest, got %+v", got)
	}
}

// assertNoPasswordLeak fails when a result prints a raw or encoded password.
func assertNoPasswordLeak(t *testing.T, results []doctorCheckResult) {
	t.Helper()
	for _, r := range results {
		for _, text := range []string{r.Message, r.Detail} {
			for _, pw := range []string{fakePGPassword, fakeRedisPassword} {
				if strings.Contains(text, pw) || strings.Contains(text, config.URLPassword(pw)) {
					t.Errorf("result leaks a password: %q", text)
				}
			}
		}
	}
}
