package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

// writeEnv is a small helper: the cascade is a set of files on disk, so these
// tests build real ones rather than mocking the reader.
func writeEnv(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestPluginEnvPassesOnlyDeclaredVariables covers the reason this exists: a
// command plugin runs on the user's machine, where nothing has read the .env
// cascade, so without help it sees no project configuration at all.
//
// It also covers the limit: only what the manifest declares is passed. A plugin
// gets what it asked for, which is visible in its manifest at install time,
// rather than everything in .env.secrets by default.
func TestPluginEnvPassesOnlyDeclaredVariables(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "BACKUP_DIR=/srv/backups\nPOSTGRES_PASSWORD=hunter2\n")

	m := &PluginManifest{EnvVars: EnvVarList{{Name: "BACKUP_DIR"}}}

	got := PluginEnv(dir, m)

	if len(got) != 1 || got[0] != "BACKUP_DIR=/srv/backups" {
		t.Fatalf("PluginEnv = %v, want exactly [BACKUP_DIR=/srv/backups]", got)
	}
	for _, e := range got {
		if e == "POSTGRES_PASSWORD=hunter2" {
			t.Error("an undeclared secret was passed to the plugin")
		}
	}
}

// TestPluginEnvFollowsTheCanonicalCascadeOrder proves the order comes from
// config.EnvCascadeOrder rather than a second copy of the rules. CLI-R18 exists
// because three implementations of this order had already drifted apart.
func TestPluginEnvFollowsTheCanonicalCascadeOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENV", "dev")

	// Canonical dev order: .env, .env.dev, .env.secrets, .env.local — later wins.
	writeEnv(t, dir, ".env", "TARGET=from-env\n")
	writeEnv(t, dir, ".env.dev", "TARGET=from-env-dev\n")
	writeEnv(t, dir, ".env.local", "TARGET=from-env-local\n")

	m := &PluginManifest{EnvVars: EnvVarList{{Name: "TARGET"}}}

	got := PluginEnv(dir, m)
	if len(got) != 1 || got[0] != "TARGET=from-env-local" {
		t.Fatalf("PluginEnv = %v, want [TARGET=from-env-local] — .env.local is last in the cascade", got)
	}
}

// TestPluginEnvDoesNotOverrideTheRealEnvironment pins the direction of
// precedence, which is deliberately the opposite of what the CLI does
// internally.
//
// config.Load uses godotenv.Overload, so inside the CLI a .env file beats the
// shell. For a child process that is the wrong way round: someone typing
// `BACKUP_DIR=/tmp nself backup ...` means it, and the file must not quietly
// win.
func TestPluginEnvDoesNotOverrideTheRealEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "BACKUP_DIR=/from/file\n")
	t.Setenv("BACKUP_DIR", "/from/shell")

	m := &PluginManifest{EnvVars: EnvVarList{{Name: "BACKUP_DIR"}}}

	if got := PluginEnv(dir, m); len(got) != 0 {
		t.Fatalf("PluginEnv = %v, want nothing added: the shell value must survive", got)
	}
}

// TestPluginEnvOutsideAProject covers running a plugin anywhere else. An absent
// value is passed as absent rather than as empty, so the plugin's own default
// applies instead of being overwritten with "".
func TestPluginEnvOutsideAProject(t *testing.T) {
	dir := t.TempDir() // no .env files at all

	m := &PluginManifest{EnvVars: EnvVarList{{Name: "BACKUP_DIR"}}}

	if got := PluginEnv(dir, m); got != nil {
		t.Errorf("PluginEnv = %v, want nil outside a project", got)
	}
}

// TestPluginEnvDeclaresNothing is the common case: most plugins declare no
// project settings and must cost nothing.
func TestPluginEnvDeclaresNothing(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "BACKUP_DIR=/srv/backups\n")

	if got := PluginEnv(dir, &PluginManifest{}); got != nil {
		t.Errorf("PluginEnv = %v, want nil when nothing is declared", got)
	}
	if got := PluginEnv(dir, nil); got != nil {
		t.Errorf("PluginEnv(nil manifest) = %v, want nil", got)
	}
}

// TestPluginEnvResolvesEnvFromProjectFileNotBareDefault pins the fix for the
// production bug this test exists to catch: with no ENV set in the process,
// resolveCascade must read the project's own .env ENV= key (via
// config.ResolveEnv, the same resolver config.Load uses) rather than bare
// os.Getenv("ENV") defaulting straight to "dev".
//
// Before the fix, a plugin-invoking command on a prod box whose only signal
// was .env's ENV=prod would read the .env.dev layer while `nself build` on
// the same box read .env.prod for the same project — two commands, two
// different configs.
func TestPluginEnvResolvesEnvFromProjectFileNotBareDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENV", "") // nothing in the process environment

	// The project's own .env carries ENV=prod, as a bare production checkout
	// does. .env.dev and .env.prod disagree on TARGET so the test can tell
	// which layer actually loaded.
	writeEnv(t, dir, ".env", "ENV=prod\nTARGET=from-env\n")
	writeEnv(t, dir, ".env.dev", "TARGET=from-env-dev\n")
	writeEnv(t, dir, ".env.prod", "TARGET=from-env-prod\n")

	m := &PluginManifest{EnvVars: EnvVarList{{Name: "TARGET"}}}

	got := PluginEnv(dir, m)
	if len(got) != 1 || got[0] != "TARGET=from-env-prod" {
		t.Fatalf("PluginEnv = %v, want [TARGET=from-env-prod] — ENV=prod in the project's .env must select the prod cascade layer, not the dev default", got)
	}
}

// TestPluginEnvDoesNotMutateThisProcess is the constraint that keeps this from
// becoming the thing it replaced. config.Load exports every value it reads into
// the CLI's own environment; this must not, or every plugin would inherit the
// whole cascade regardless of what it declared.
func TestPluginEnvDoesNotMutateThisProcess(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "SHOULD_NOT_LEAK=1\nBACKUP_DIR=/srv/backups\n")

	m := &PluginManifest{EnvVars: EnvVarList{{Name: "BACKUP_DIR"}}}
	_ = PluginEnv(dir, m)

	if v, ok := os.LookupEnv("SHOULD_NOT_LEAK"); ok {
		t.Errorf("reading the cascade leaked %q=%q into this process", "SHOULD_NOT_LEAK", v)
	}
	if v, ok := os.LookupEnv("BACKUP_DIR"); ok {
		t.Errorf("reading the cascade leaked %q=%q into this process", "BACKUP_DIR", v)
	}
}
