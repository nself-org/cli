package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// ── EnvCascadeOrder (pure, table-driven) ────────────────────────────────────

// TestEnvCascadeOrder_Canonical verifies the CLI-R18 canonical order for every
// environment name: .env → .env.{env} → .env.secrets → .env.local, with no
// .env.ai layer at all.
func TestEnvCascadeOrder_Canonical(t *testing.T) {
	cases := []struct {
		env  string
		want []string
	}{
		{"dev", []string{".env", ".env.dev", ".env.secrets", ".env.local"}},
		{"staging", []string{".env", ".env.staging", ".env.secrets", ".env.local"}},
		{"prod", []string{".env", ".env.prod", ".env.secrets", ".env.local"}},
		{"production", []string{".env", ".env.prod", ".env.secrets", ".env.local"}},
		{"development", []string{".env", ".env.dev", ".env.secrets", ".env.local"}},
		{"stage", []string{".env", ".env.staging", ".env.secrets", ".env.local"}},
		{"PROD", []string{".env", ".env.prod", ".env.secrets", ".env.local"}},
		// "local" is the dev stack on this machine.
		{"local", []string{".env", ".env.dev", ".env.secrets", ".env.local"}},
		// A custom env layers its own file over the dev base (P7-DEPL-12).
		{"test", []string{".env", ".env.test", ".env.secrets", ".env.local"}},
		{"QA", []string{".env", ".env.qa", ".env.secrets", ".env.local"}},
		{"live", []string{".env", ".env.live", ".env.secrets", ".env.local"}},
		{"qa-eu_2", []string{".env", ".env.qa-eu_2", ".env.secrets", ".env.local"}},
		// A name that is not a plain file-name fragment gets no layer of its own.
		{"../prod", []string{".env", ".env.secrets", ".env.local"}},
		{"a/b", []string{".env", ".env.secrets", ".env.local"}},
		{" prod", []string{".env", ".env.secrets", ".env.local"}},
	}

	for _, c := range cases {
		got := EnvCascadeOrder(c.env, false)
		if !equalStrings(got, c.want) {
			t.Errorf("EnvCascadeOrder(%q, false) = %v, want %v", c.env, got, c.want)
		}
	}
}

// TestEnvCascadeOrder_Legacy verifies the pre-CLI-R18 order is preserved
// byte-for-byte for the NSELF_LEGACY_ENV_ORDER escape hatch: .env.dev is
// always the base layer, .env and .env.ai win last.
func TestEnvCascadeOrder_Legacy(t *testing.T) {
	cases := []struct {
		env  string
		want []string
	}{
		{"dev", []string{".env.dev", ".env.secrets", ".env.local", ".env", ".env.ai"}},
		{"staging", []string{".env.dev", ".env.staging", ".env.secrets", ".env.local", ".env", ".env.ai"}},
		{"prod", []string{".env.dev", ".env.prod", ".env.secrets", ".env.local", ".env", ".env.ai"}},
		// Unknown env names still get the always-loaded .env.dev base — the
		// historical quirk this reorder removes.
		{"test", []string{".env.dev", ".env.secrets", ".env.local", ".env", ".env.ai"}},
	}

	for _, c := range cases {
		got := EnvCascadeOrder(c.env, true)
		if !equalStrings(got, c.want) {
			t.Errorf("EnvCascadeOrder(%q, true) = %v, want %v", c.env, got, c.want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── EnvCascade (existence resolution) ───────────────────────────────────────

// TestEnvCascade_ResolvesExistence verifies EnvCascade reports which cascade
// files actually exist on disk without requiring all of them to be present.
func TestEnvCascade_ResolvesExistence(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".env"), "X=1\n")
	mustWriteFile(t, filepath.Join(dir, ".env.local"), "X=2\n")

	files := EnvCascade(dir, "dev", false)
	want := map[string]bool{
		".env":         true,
		".env.dev":     false,
		".env.secrets": false,
		".env.local":   true,
	}
	if len(files) != len(want) {
		t.Fatalf("EnvCascade returned %d entries, want %d", len(files), len(want))
	}
	for _, f := range files {
		exp, ok := want[f.Name]
		if !ok {
			t.Errorf("unexpected cascade file %q", f.Name)
			continue
		}
		if f.Exists != exp {
			t.Errorf("EnvCascade(%q).Exists = %v, want %v", f.Name, f.Exists, exp)
		}
		if f.Path != filepath.Join(dir, f.Name) {
			t.Errorf("EnvCascade(%q).Path = %q, want %q", f.Name, f.Path, filepath.Join(dir, f.Name))
		}
	}
}

// ── Load() precedence — canonical order ─────────────────────────────────────

const cascadeTestVar = "NSELF_TEST_CASCADE_VAR"

// TestLoad_CanonicalPrecedence_LocalBeatsSecretsBeatsEnvBeatsBase verifies the
// full new-order precedence chain end-to-end through Load(): .env.local beats
// .env.secrets beats .env.dev beats bare .env. Table-driven: each case adds
// one more, higher-precedence file and checks the winner shifts.
func TestLoad_CanonicalPrecedenceChain(t *testing.T) {
	_ = os.Unsetenv("ENV")
	_ = os.Unsetenv(LegacyEnvOrderVar)
	t.Cleanup(func() { _ = os.Unsetenv(cascadeTestVar) })

	dir := t.TempDir()

	// Step 1: only bare .env — it wins by default.
	mustWriteFile(t, filepath.Join(dir, ".env"), cascadeTestVar+"=from-env\n")
	if _, err := Load(dir); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := os.Getenv(cascadeTestVar); got != "from-env" {
		t.Errorf("after .env only: got %q, want %q", got, "from-env")
	}

	// Step 2: add .env.dev — it should now beat bare .env.
	mustWriteFile(t, filepath.Join(dir, ".env.dev"), cascadeTestVar+"=from-dev\n")
	if _, err := Load(dir); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := os.Getenv(cascadeTestVar); got != "from-dev" {
		t.Errorf("after .env.dev added: got %q, want %q", got, "from-dev")
	}

	// Step 3: add .env.secrets — it should now beat .env.dev.
	mustWriteFile(t, filepath.Join(dir, ".env.secrets"), cascadeTestVar+"=from-secrets\n")
	if _, err := Load(dir); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := os.Getenv(cascadeTestVar); got != "from-secrets" {
		t.Errorf("after .env.secrets added: got %q, want %q", got, "from-secrets")
	}

	// Step 4: add .env.local — it should now beat .env.secrets (highest).
	mustWriteFile(t, filepath.Join(dir, ".env.local"), cascadeTestVar+"=from-local\n")
	if _, err := Load(dir); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := os.Getenv(cascadeTestVar); got != "from-local" {
		t.Errorf("after .env.local added: got %q, want %q", got, "from-local")
	}
}

// TestLoad_CanonicalOrder_EnvAiIsIgnored verifies that a leftover .env.ai file
// (pre-CLI-R18 projects) is no longer part of the cascade at all under the
// canonical order: it must not win even though it used to be loaded last.
func TestLoad_CanonicalOrder_EnvAiIsIgnored(t *testing.T) {
	_ = os.Unsetenv("ENV")
	_ = os.Unsetenv(LegacyEnvOrderVar)
	t.Cleanup(func() { _ = os.Unsetenv(cascadeTestVar) })

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".env"), cascadeTestVar+"=from-env\n")
	mustWriteFile(t, filepath.Join(dir, ".env.ai"), cascadeTestVar+"=from-ai\n")

	if _, err := Load(dir); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := os.Getenv(cascadeTestVar); got != "from-env" {
		t.Errorf("got %q, want %q (.env.ai must not be consulted)", got, "from-env")
	}
}

// ── Load() ENV resolution — process env vs .env's own ENV= key ─────────────

// TestLoad_EnvResolvedFromDotEnvFile_WhenProcessEnvUnset verifies the
// production-server bug fix directly: with no ENV set in the process
// environment, Load() must read ENV=prod out of the project's own .env file
// and load .env.prod — not silently default to dev. This is the exact
// shape of a bare `nself build` run on a checked-out prod project.
func TestLoad_EnvResolvedFromDotEnvFile_WhenProcessEnvUnset(t *testing.T) {
	_ = os.Unsetenv("ENV")
	_ = os.Unsetenv(LegacyEnvOrderVar)
	t.Cleanup(func() { _ = os.Unsetenv(cascadeTestVar) })

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".env"), "ENV=prod\n")
	// Satisfy the prod-only MinIO strong-credential guard (T08) so Load()
	// exercises the full pipeline through ApplyDefaults, not just cascade
	// file selection.
	mustWriteFile(t, filepath.Join(dir, ".env.prod"), cascadeTestVar+"=from-env-prod\n"+
		"MINIO_ROOT_USER=prod-minio-user\nMINIO_ROOT_PASSWORD=a-strong-unique-password-16plus\n")

	if _, err := Load(dir); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := os.Getenv(cascadeTestVar); got != "from-env-prod" {
		t.Errorf("got %q, want %q (.env's ENV=prod must select .env.prod)", got, "from-env-prod")
	}
}

// TestLoad_ProcessEnv_WinsOverDotEnvFile verifies that an ENV already set in
// the process environment is never overridden by .env's own ENV= key — the
// process environment is the higher-priority signal (e.g. CI exporting
// ENV=staging for a project whose .env still says ENV=prod).
func TestLoad_ProcessEnv_WinsOverDotEnvFile(t *testing.T) {
	_ = os.Setenv("ENV", "dev")
	_ = os.Unsetenv(LegacyEnvOrderVar)
	t.Cleanup(func() {
		_ = os.Unsetenv("ENV")
		_ = os.Unsetenv(cascadeTestVar)
	})

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".env"), "ENV=prod\n")
	mustWriteFile(t, filepath.Join(dir, ".env.dev"), cascadeTestVar+"=from-env-dev\n")
	mustWriteFile(t, filepath.Join(dir, ".env.prod"), cascadeTestVar+"=from-env-prod\n")

	if _, err := Load(dir); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := os.Getenv(cascadeTestVar); got != "from-env-dev" {
		t.Errorf("got %q, want %q (process ENV=dev must win over .env's ENV=prod)", got, "from-env-dev")
	}
}

// ── Load() — legacy escape hatch ────────────────────────────────────────────

// TestLoad_LegacyOrder_EnvAiWinsLast verifies that with NSELF_LEGACY_ENV_ORDER
// set, the historical order is restored: bare .env and then .env.ai win last,
// overriding .env.secrets and .env.local exactly as before CLI-R18.
func TestLoad_LegacyOrder_EnvAiWinsLast(t *testing.T) {
	_ = os.Unsetenv("ENV")
	_ = os.Setenv(LegacyEnvOrderVar, "1")
	t.Cleanup(func() {
		_ = os.Unsetenv(LegacyEnvOrderVar)
		_ = os.Unsetenv(cascadeTestVar)
	})

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".env.secrets"), cascadeTestVar+"=from-secrets\n")
	mustWriteFile(t, filepath.Join(dir, ".env.local"), cascadeTestVar+"=from-local\n")
	mustWriteFile(t, filepath.Join(dir, ".env"), cascadeTestVar+"=from-env\n")
	mustWriteFile(t, filepath.Join(dir, ".env.ai"), cascadeTestVar+"=from-ai\n")

	if _, err := Load(dir); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := os.Getenv(cascadeTestVar); got != "from-ai" {
		t.Errorf("got %q, want %q (.env.ai must win under legacy order)", got, "from-ai")
	}
}

// TestLoad_LegacyOrder_WarnsOnEveryUse verifies that setting
// NSELF_LEGACY_ENV_ORDER emits a slog.Warn naming the variable and a removal
// version on every single Load() call, not just the first.
func TestLoad_LegacyOrder_WarnsOnEveryUse(t *testing.T) {
	_ = os.Unsetenv("ENV")
	_ = os.Setenv(LegacyEnvOrderVar, "1")
	t.Cleanup(func() { _ = os.Unsetenv(LegacyEnvOrderVar) })

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".env"), "X=1\n")

	h := &captureHandler{level: slog.LevelWarn}
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(orig) })

	for i := 0; i < 2; i++ {
		if _, err := Load(dir); err != nil {
			t.Fatalf("Load() error: %v", err)
		}
	}

	var warnings int
	for _, r := range h.records {
		attrs := attrMapCascade(r)
		if attrs["var"] == LegacyEnvOrderVar {
			warnings++
			if attrs["removed_in"] == "" {
				t.Error("legacy-order warning missing removed_in attr")
			}
		}
	}
	if warnings != 2 {
		t.Errorf("expected 2 legacy-order warnings (one per Load call), got %d", warnings)
	}
}

// ── test helpers ─────────────────────────────────────────────────────────────

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// captureHandler records slog.Records emitted during a test. Mirrors the
// pattern in internal/secrets/secrets_slog_test.go.
type captureHandler struct {
	records []slog.Record
	level   slog.Level
}

func (h *captureHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(name string) slog.Handler       { return h }

func attrMapCascade(r slog.Record) map[string]string {
	m := make(map[string]string)
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.String()
		return true
	})
	return m
}

// TestLoadCustomEnvNeverReadsDevLayer: a server whose ENV is a custom name
// (live) builds from .env, .env.live, .env.secrets and .env.local only; the
// dev values in a committed .env.dev never reach it. A dev name still loads it.
func TestLoadCustomEnvNeverReadsDevLayer(t *testing.T) {
	for _, tc := range []struct {
		env     string
		wantDev bool
	}{{"live", false}, {"prd", false}, {" prod", false}, {"dev", true}, {"local", true}} {
		dir := t.TempDir()
		for _, k := range []string{"HASURA_GRAPHQL_DEV_MODE", "BASE_DOMAIN", "NSELF_LEGACY_ENV_ORDER"} {
			t.Setenv(k, "")
		}
		t.Setenv("ENV", tc.env)
		for n, b := range map[string]string{
			".env":      "BASE_DOMAIN=example.com\n",
			".env.dev":  "HASURA_GRAPHQL_DEV_MODE=true\nBASE_DOMAIN=local.nself.org\n",
			".env.live": "X_LIVE=1\n",
		} {
			if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("ENV=%q: %v", tc.env, err)
		}
		gotDev := os.Getenv("HASURA_GRAPHQL_DEV_MODE") == "true" || cfg.BaseDomain == "local.nself.org"
		if gotDev != tc.wantDev {
			t.Errorf("ENV=%q: dev values loaded=%v, want %v (DEV_MODE=%q BASE_DOMAIN=%q)", tc.env, gotDev, tc.wantDev, os.Getenv("HASURA_GRAPHQL_DEV_MODE"), cfg.BaseDomain)
		}
	}
}

// TestLoadRemoteDeployDropsEnvLocal: after SetRemoteCascade(true) (the hidden
// build flag nself deploy passes for a remote target) Load skips .env.local;
// otherwise the personal override still wins.
func TestLoadRemoteDeployDropsEnvLocal(t *testing.T) {
	for _, remote := range []bool{false, true} {
		dir := t.TempDir()
		t.Setenv("ENV", "qa")
		t.Setenv("BASE_DOMAIN", "")
		t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
		SetRemoteCascade(remote)
		t.Cleanup(func() { SetRemoteCascade(false) })
		for n, b := range map[string]string{".env.qa": "BASE_DOMAIN=qa.example.test\n", ".env.local": "BASE_DOMAIN=laptop.example.test\n"} {
			if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		cfg, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		want := "laptop.example.test"
		if remote {
			want = "qa.example.test"
		}
		if cfg.BaseDomain != want {
			t.Errorf("remote=%v: BASE_DOMAIN=%q, want %q", remote, cfg.BaseDomain, want)
		}
	}
	if got := WithoutLocalOverride([]string{".env", ".env.local", ".env.prod"}); len(got) != 2 || got[0] != ".env" || got[1] != ".env.prod" {
		t.Errorf("WithoutLocalOverride = %v", got)
	}
}

// TestExportedRemoteVarHasNoEffect: a user-exported NSELF_DEPLOY_REMOTE=true
// changes nothing; only the caller's SetRemoteCascade does.
func TestExportedRemoteVarHasNoEffect(t *testing.T) {
	SetRemoteCascade(false)
	dir := t.TempDir()
	t.Setenv("ENV", "qa")
	t.Setenv("BASE_DOMAIN", "")
	t.Setenv("NSELF_DEPLOY_REMOTE", "true")
	for n, b := range map[string]string{".env.qa": "BASE_DOMAIN=qa.example.test\n", ".env.local": "BASE_DOMAIN=laptop.example.test\n"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseDomain != "laptop.example.test" {
		t.Errorf("exported NSELF_DEPLOY_REMOTE changed the cascade: BASE_DOMAIN=%q", cfg.BaseDomain)
	}
}
