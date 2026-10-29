package config

// env_protected_test.go — P7-PLUG-87 review H1: a cloned project's env files
// cannot set a security knob. Every cascade layer, the overlay path and
// OverloadEnvFile are covered for every protected key, with a positive
// control, and a source sweep keeps new trust-path knobs classified.

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/compat"
)

// attackValue is what a hostile project writes for every protected key.
const attackValue = "https://attacker.example"

// protectedProject writes name holding every protected key plus a control key,
// isolates those keys in the process env, and returns the project dir.
func protectedProject(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("PROJECT_NAME=h1\nENV=dev\nH1_CONTROL=exported\n")
	for _, k := range ProtectedEnvKeys() {
		b.WriteString(k + "=" + attackValue + "\n")
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	for _, k := range []string{"ENV", "PROJECT_NAME", "H1_CONTROL", compat.EnvVar} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// captureWarnings routes slog to a buffer and resets the once-per-key memory.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	protectedWarned = sync.Map{}
	t.Cleanup(func() { slog.SetDefault(orig); protectedWarned = sync.Map{} })
	return &buf
}

func assertProtectedUnset(t *testing.T) {
	t.Helper()
	for _, k := range ProtectedEnvKeys() {
		if v, ok := os.LookupEnv(k); ok {
			t.Errorf("%s = %q after load: a project file set a protected key", k, v)
		}
	}
	if os.Getenv("H1_CONTROL") != "exported" {
		t.Fatal("control key not exported: the test does not exercise the loader")
	}
}

func TestProjectEnvCannotSetProtectedKeys(t *testing.T) {
	for _, name := range []string{".env", ".env.dev", ".env.secrets", ".env.local"} {
		t.Run(name, func(t *testing.T) {
			logs := captureWarnings(t)
			dir := protectedProject(t, name)
			if _, err := Load(dir); err != nil {
				t.Fatal(err)
			}
			assertProtectedUnset(t)
			out := logs.String()
			for _, k := range ProtectedEnvKeys() {
				if !strings.Contains(out, "var="+k) {
					t.Errorf("no warning names %s", k)
				}
			}
			if strings.Contains(out, attackValue) {
				t.Fatal("the warning must not log the value")
			}
		})
	}
}

func TestProjectEnvKeepsOperatorValue(t *testing.T) {
	captureWarnings(t)
	dir := protectedProject(t, ".env")
	t.Setenv("NSELF_PLUGIN_REGISTRY", "https://plugins.nself.org")
	t.Setenv("NSELF_PLUGIN_REQUIRE_CHECKSUM", "1")
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("NSELF_PLUGIN_REGISTRY"); got != "https://plugins.nself.org" {
		t.Fatalf("registry = %q, want the operator's value", got)
	}
	if got := os.Getenv("NSELF_PLUGIN_REQUIRE_CHECKSUM"); got != "1" {
		t.Fatalf("require-checksum = %q, want the operator's 1", got)
	}
}

func TestOverlayCannotSetProtectedKeys(t *testing.T) {
	captureWarnings(t)
	dir := protectedProject(t, ".env.unused")
	data, err := os.ReadFile(filepath.Join(dir, ".env.unused"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithOptions(dir, LoadOptions{EnvOverlay: map[string][]byte{".env.secrets": data}}); err != nil {
		t.Fatal(err)
	}
	assertProtectedUnset(t)
}

func TestOverloadEnvFileSkipsProtectedKeys(t *testing.T) {
	captureWarnings(t)
	dir := protectedProject(t, ".env.prod")
	if err := OverloadEnvFile(filepath.Join(dir, ".env.prod")); err != nil {
		t.Fatal(err)
	}
	assertProtectedUnset(t)
	if err := OverloadEnvFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("a missing file must be an error")
	}
}

// TestProjectEnvCannotTurnOffV15: a project may opt into v1.5 but never turn
// off an opt-in the process already has.
func TestProjectEnvCannotTurnOffV15(t *testing.T) {
	captureWarnings(t)
	dir := protectedProject(t, ".env.local")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(compat.EnvVar+"=0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(compat.EnvVar, "1")
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	if !compat.V15() {
		t.Fatal("a project .env turned off the operator's NSELF_V15=1")
	}
	_ = os.Unsetenv(compat.EnvVar)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(compat.EnvVar+"=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	if !compat.V15() {
		t.Fatal("a project .env may still opt into v1.5")
	}
}

func TestExplainProtectedKeyHasNoFileWinner(t *testing.T) {
	dir := protectedProject(t, ".env")
	t.Setenv("NSELF_PLUGIN_REGISTRY", "https://plugins.nself.org")
	exp, err := ExplainKey(dir, "dev", "NSELF_PLUGIN_REGISTRY")
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Protected || len(exp.Setters) != 1 {
		t.Fatalf("want protected with one setter, got %+v", exp)
	}
	if _, _, ok := exp.Winner(); ok {
		t.Fatal("a protected key must have no file winner")
	}
	if r := exp.NoWinnerReason(); !strings.Contains(r, "ignored") {
		t.Fatalf("reason = %q, want it to say the file is ignored", r)
	}
	if src, v := exp.Effective(); src != "process environment" || v != "https://plugins.nself.org" {
		t.Fatalf("effective = %s %q", src, v)
	}
}

// projectSettableKeys are NSELF_* keys the trust-path packages read that a
// project may set, each reviewed: none turns a verification off or moves a
// trust endpoint. A new key there must land here or in protectedEnvKeys.
var projectSettableKeys = map[string]string{
	"NSELF_BUNDLES_CACHE_PATH":       "cache location; cached catalogues are still verified",
	"NSELF_CLI_VERSION":              "set by the CLI for plugins, not read for trust",
	"NSELF_ENTITLEMENT_AUTOREFRESH":  "refresh scheduling only",
	"NSELF_ENV":                      "only makes owner-key saving stricter",
	"NSELF_OWNER_LICENSE_KEY":        "a licence key; the ping service validates it",
	"NSELF_PLUGIN_CACHE":             "cache location; cached entries are still verified",
	"NSELF_PLUGIN_DIR":               "install location; a project-level setting",
	"NSELF_PLUGIN_LICENSE_KEY":       "the project's licence key, kept in .env.secrets",
	"NSELF_PLUGIN_LICENSE_KEY_OWNER": "a licence key; the ping service validates it",
	"NSELF_PLUGIN_READY_TIMEOUT":     "a wait bound",
}

func TestTrustPathEnvKeysClassified(t *testing.T) {
	re := regexp.MustCompile(`"(NSELF_[A-Z0-9_]+)"`)
	seen := map[string]bool{}
	for _, root := range []string{"../plugin", "../license", "../bundle"} {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, err := os.ReadFile(p)
			for _, m := range re.FindAllSubmatch(b, -1) {
				seen[string(m[1])] = true
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !seen["NSELF_LICENSE_SKIP_VERIFY"] {
		t.Fatal("sweep found nothing: the source paths are wrong")
	}
	var unclassified []string
	for k := range seen {
		if !IsProtectedEnvKey(k) && projectSettableKeys[k] == "" {
			unclassified = append(unclassified, k)
		}
	}
	sort.Strings(unclassified)
	if len(unclassified) > 0 {
		t.Fatalf("classify these trust-path env keys (protectedEnvKeys or projectSettableKeys): %v", unclassified)
	}
}
