package build

// orchestrator_secrets_adopt_test.go — D-0506: adopt the live credential from
// the previous build, warn on shadowing, and the plan-mode path. Fake values.

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/composeenv"
	"github.com/nself-org/cli/internal/config"
)

func genCfg() *config.Config {
	return &config.Config{
		Postgres: config.PostgresConfig{Password: "fake-generated-pg-password"},
		Hasura:   config.HasuraConfig{AdminSecret: "fake-generated-admin-secret", JWTKey: "fake-jwt-key-for-test", JWTType: "HS256"},
	}
}

func projectWithComposeEnv(t *testing.T, composeEnv string) string {
	t.Helper()
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n", 0600)
	testWriteFile(t, filepath.Join(dir, ".nself", "compose.env"), composeEnv, 0600)
	return dir
}

// Adoption is for this build's render only: cfg gets the live value, the key is
// NOT written to .env.secrets (compose.env cannot tell a generated value from a
// CI export), and the next build adopts the same value again.
func TestPersistGeneratedSecrets_AdoptsLiveCredentialsWithoutPersisting(t *testing.T) {
	dir := projectWithComposeEnv(t, "# GENERATED\nPOSTGRES_PASSWORD=fake-live-pg-password\nHASURA_GRAPHQL_ADMIN_SECRET='fake-live$admin-secret'\n")
	first := buildOnce(t, dir)
	if first.Postgres.Password != "fake-live-pg-password" || first.Hasura.AdminSecret != "fake-live$admin-secret" {
		t.Fatal("cfg did not adopt the live credentials")
	}
	data := readSecrets(t, dir)
	for _, key := range []string{"POSTGRES_PASSWORD=", "HASURA_GRAPHQL_ADMIN_SECRET="} {
		if strings.Contains(data, key) {
			t.Errorf("adopted %s was written to .env.secrets", key)
		}
	}
	second := buildOnce(t, dir)
	if second.Postgres.Password != first.Postgres.Password || second.Hasura.AdminSecret != first.Hasura.AdminSecret {
		t.Error("second build did not adopt the same values")
	}
}

// The CI-export repro: export A -> compose.env A; a build without the export
// renders A and writes nothing; a build with the rotated export B renders B.
func TestPersistGeneratedSecrets_RotatedExportStillWinsAfterAdoption(t *testing.T) {
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n", 0600)
	composeFor := func(pw string) {
		testWriteFile(t, filepath.Join(dir, ".nself", "compose.env"), "POSTGRES_PASSWORD="+pw+"\n", 0600)
	}
	t.Setenv("POSTGRES_PASSWORD", "fake-ci-A")
	if got := buildOnce(t, dir).Postgres.Password; got != "fake-ci-A" {
		t.Fatalf("build 1 with export A rendered %q", got)
	}
	composeFor("fake-ci-A") // what build 1's render left behind
	_ = os.Unsetenv("POSTGRES_PASSWORD")
	if got := buildOnce(t, dir).Postgres.Password; got != "fake-ci-A" {
		t.Fatalf("build 2 without export rendered %q, want A", got)
	}
	if strings.Contains(readSecrets(t, dir), "POSTGRES_PASSWORD=") {
		t.Fatal("build 2 wrote the adopted value to .env.secrets; a rotated export would be shadowed")
	}
	t.Setenv("POSTGRES_PASSWORD", "fake-ci-B")
	if got := buildOnce(t, dir).Postgres.Password; got != "fake-ci-B" {
		t.Fatalf("build 3 with rotated export B rendered %q", got)
	}
}

func TestPersistGeneratedSecrets_ComposeEnvReferenceIgnored(t *testing.T) {
	dir := projectWithComposeEnv(t, "POSTGRES_PASSWORD=${POSTGRES_PASSWORD}\nHASURA_GRAPHQL_ADMIN_SECRET=\n")
	cfg := genCfg()
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Password != "fake-generated-pg-password" || cfg.Hasura.AdminSecret != "fake-generated-admin-secret" {
		t.Fatal("a reference or empty value must not be adopted")
	}
	data := readSecrets(t, dir)
	if !strings.Contains(data, "POSTGRES_PASSWORD=fake-generated-pg-password\n") {
		t.Errorf("generated value not persisted:\n%s", data)
	}
}

func TestPersistGeneratedSecrets_AdoptNotWhenProcessEnvOwnsKey(t *testing.T) {
	dir := projectWithComposeEnv(t, "POSTGRES_PASSWORD=fake-live-pg-password\n")
	t.Setenv("POSTGRES_PASSWORD", "fake-ci-exported-password")
	cfg := genCfg()
	cfg.Postgres.Password = "fake-ci-exported-password"
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Password != "fake-ci-exported-password" || strings.Contains(readSecrets(t, dir), "POSTGRES_PASSWORD=") {
		t.Error("process-env-owned value was replaced or persisted")
	}
}

// captureWarnings runs persist on a project with the given env files and
// returns what slog wrote.
func captureWarnings(t *testing.T, env string, files map[string]string) string {
	t.Helper()
	clearSecretEnv(t)
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	dir := t.TempDir()
	for name, body := range files {
		testWriteFile(t, filepath.Join(dir, name), body, 0600)
	}
	cfg := genCfg()
	cfg.Env = env
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatal(err)
	}
	return logs.String()
}

const shadowMsg = "wins because it loads last"

func TestPersistGeneratedSecrets_WarnsWhenSecretsFileShadowsEarlierFile(t *testing.T) {
	for _, earlier := range []string{".env.prod", ".env"} { // both load before .env.secrets (canonical)
		out := captureWarnings(t, "prod", map[string]string{
			earlier:        "POSTGRES_PASSWORD=fake-earlier-password\nCRON_INTERNAL_SECRET=same-value\n",
			".env.secrets": "POSTGRES_PASSWORD=fake-secrets-password\nCRON_INTERNAL_SECRET=same-value\n",
		})
		if n := strings.Count(out, shadowMsg); n != 1 {
			t.Fatalf("%s: got %d shadow warnings, want 1 (equal values must not warn):\n%s", earlier, n, out)
		}
		if !strings.Contains(out, "POSTGRES_PASSWORD") || !strings.Contains(out, earlier) {
			t.Errorf("%s: warning must name the key and file:\n%s", earlier, out)
		}
		if strings.Contains(out, "fake-earlier-password") || strings.Contains(out, "fake-secrets-password") {
			t.Error("warning leaked a value")
		}
	}
}

// .env.local loads AFTER .env.secrets, so it wins: nothing is shadowed.
func TestPersistGeneratedSecrets_NoWarningForFileLoadedAfterSecrets(t *testing.T) {
	out := captureWarnings(t, "dev", map[string]string{
		".env.local":   "POSTGRES_PASSWORD=fake-local-password\n",
		".env.secrets": "POSTGRES_PASSWORD=fake-secrets-password\n",
	})
	if strings.Contains(out, shadowMsg) {
		t.Errorf(".env.local beats .env.secrets; no warning expected:\n%s", out)
	}
}

// A file outside the active cascade (.env.prod while ENV=dev) never loads.
func TestPersistGeneratedSecrets_NoWarningForInactiveEnvFile(t *testing.T) {
	out := captureWarnings(t, "dev", map[string]string{
		".env.prod":    "POSTGRES_PASSWORD=fake-prod-password\n",
		".env.secrets": "POSTGRES_PASSWORD=fake-secrets-password\n",
	})
	if strings.Contains(out, shadowMsg) {
		t.Errorf("inactive .env.prod is not loaded; no warning expected:\n%s", out)
	}
}

// Plan mode: the planned .env.secrets carries the generated key, and nothing is
// written to disk.
func TestPersistGeneratedSecrets_PlanModeWritesNothingToDisk(t *testing.T) {
	dir := projectWithComposeEnv(t, "POSTGRES_PASSWORD=fake-live-pg-password\n")
	sink := newMemSink(dir, "")
	fx := &planEffects{}
	cfg := genCfg()
	if err := persistGeneratedSecretsFx(dir, cfg, fx, sink); err != nil {
		t.Fatal(err)
	}
	planned, err := sink.ReadFile(filepath.Join(dir, ".env.secrets"))
	if err != nil {
		t.Fatalf("planned .env.secrets: %v", err)
	}
	// The admin secret is generated (persisted); the Postgres password is adopted
	// from compose.env (render only, never written).
	if !strings.Contains(string(planned), "HASURA_GRAPHQL_ADMIN_SECRET=fake-generated-admin-secret\n") {
		t.Errorf("planned bytes lack the generated admin secret:\n%s", planned)
	}
	if strings.Contains(string(planned), "POSTGRES_PASSWORD=") || cfg.Postgres.Password != "fake-live-pg-password" {
		t.Errorf("adopted Postgres password must be used but not planned:\n%s", planned)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.secrets")); !os.IsNotExist(err) {
		t.Errorf(".env.secrets reached disk in plan mode (stat err = %v)", err)
	}
}

// Custom env files (.env.qa) are part of the scan: a dev build must not write a
// generated value into the shared .env.secrets while .env.qa owns the key.
func TestPersistGeneratedSecrets_CustomEnvFileKeyNotPersisted(t *testing.T) {
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\nENV=dev\n", 0600)
	testWriteFile(t, filepath.Join(dir, ".env.qa"), "POSTGRES_PASSWORD=fake-qa-owned-password\n", 0600)
	buildOnce(t, dir)
	if strings.Contains(readSecrets(t, dir), "POSTGRES_PASSWORD=") {
		t.Fatal("POSTGRES_PASSWORD was persisted although .env.qa defines it; it would shadow qa")
	}
}

// The shadow warning considers a custom file when it precedes .env.secrets in
// the active cascade (ENV=qa), and not otherwise.
func TestPersistGeneratedSecrets_ShadowWarningCoversCustomEnvFile(t *testing.T) {
	files := map[string]string{
		".env.qa":      "POSTGRES_PASSWORD=fake-qa-password\n",
		".env.secrets": "POSTGRES_PASSWORD=fake-secrets-password\n",
	}
	if out := captureWarnings(t, "qa", files); !strings.Contains(out, shadowMsg) || !strings.Contains(out, ".env.qa") {
		t.Errorf("ENV=qa: expected a warning naming .env.qa:\n%s", out)
	}
	if out := captureWarnings(t, "dev", files); strings.Contains(out, shadowMsg) {
		t.Errorf("ENV=dev: .env.qa is not loaded; no warning expected:\n%s", out)
	}
}

// secretScanList asks config.EnvCascadeOrder which .env.<name> files cascade, so
// it cannot drift from cascade.go: pin a few names.
func TestSecretScanList_FollowsCascadeRule(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{".env.qa", ".env.live", ".env.example", ".env.computed", ".env.QA", ".env.bad name", ".env.-x", ".envrc"} {
		testWriteFile(t, filepath.Join(dir, n), "A=1\n", 0600)
	}
	got := strings.Join(secretScanList(dir, newDiskSink(dir)), " ")
	for _, want := range []string{".env.qa", ".env.live", ".env.secrets", ".env.prod"} {
		if !strings.Contains(" "+got+" ", " "+want+" ") {
			t.Errorf("scan list lacks %s: %s", want, got)
		}
	}
	for _, bad := range []string{".env.example", ".env.computed", ".env.QA", ".env.bad name", ".env.-x", ".envrc"} {
		if strings.Contains(" "+got+" ", " "+bad+" ") {
			t.Errorf("scan list must not include %s: %s", bad, got)
		}
	}
}

// D-1149: compose.env written by a dev build must not hand its password to a
// prod build; the same env still adopts, and a file without the marker (built
// before it existed) keeps adopting.
func TestPersistGeneratedSecrets_AdoptHonoursBuildEnvMarker(t *testing.T) {
	const body = "POSTGRES_PASSWORD=fake-dev-pg-password\nHASURA_GRAPHQL_ADMIN_SECRET=fake-dev-admin-secret\n"
	for _, tc := range []struct {
		name, header, env string
		adopt             bool
	}{
		{"dev file, prod build", composeenv.Marker + "dev\n", "prod", false},
		{"prod file, dev build", composeenv.Marker + "prod\n", "dev", false},
		{"prod file, prod build", composeenv.Marker + "prod\n", "prod", true},
		{"dev file, unset env is dev", composeenv.Marker + "dev\n", "", true},
		{"no marker", "", "prod", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := projectWithComposeEnv(t, tc.header+body)
			cfg := genCfg()
			cfg.Env = tc.env
			if err := persistGeneratedSecrets(dir, cfg); err != nil {
				t.Fatal(err)
			}
			adopted := cfg.Postgres.Password == "fake-dev-pg-password" && cfg.Hasura.AdminSecret == "fake-dev-admin-secret"
			if adopted != tc.adopt {
				t.Fatalf("adopted = %v, want %v", adopted, tc.adopt)
			}
			if !tc.adopt && !strings.Contains(readSecrets(t, dir), "POSTGRES_PASSWORD=fake-generated-pg-password\n") {
				t.Error("a refused adoption must fall back to persisting the generated value")
			}
		})
	}
}

// The writer records the ENV that the reader checks.
func TestWriteComposeEnv_RecordsBuildEnv(t *testing.T) {
	dir := t.TempDir()
	cfg := genCfg()
	cfg.Env = "prod"
	cfg.ProjectName = "test"
	sink := newDiskSink(dir)
	if err := writeComposeEnvVia(sink, dir, cfg, map[string]string{"POSTGRES_PASSWORD": "fake-pg"}, nil); err != nil {
		t.Fatal(err)
	}
	if got, ok := adoptLiveCredential(sink, dir, "prod", "POSTGRES_PASSWORD"); !ok || got != "fake-pg" {
		t.Errorf("same env: got %q, %v", got, ok)
	}
	if _, ok := adoptLiveCredential(sink, dir, "dev", "POSTGRES_PASSWORD"); ok {
		t.Error("dev build adopted a prod compose.env")
	}
}
