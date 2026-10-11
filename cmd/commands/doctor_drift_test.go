package commands

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

func doctorTreeHash(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%s\x00", rel, body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestDoctorGeneratedDriftFixPlanIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := doctorDriftRequest(dir)
	req.Seed = []byte("doctor-seed")
	if _, err := reconcile.Apply(context.Background(), req, reconcile.ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "nginx", "nginx.conf")
	if err := os.Remove(conf); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(dir, "docker-compose.yml")
	f, err := os.OpenFile(compose, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n# drift\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	before := doctorTreeHash(t, dir)
	r, p, err := checkGeneratedDrift(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" || !strings.Contains(r.Message, "nginx/nginx.conf") || !strings.Contains(r.Message, "docker-compose.yml (hand-edited)") {
		t.Fatalf("drift result: %+v", r)
	}
	var rendered strings.Builder
	planCmd := &cobra.Command{}
	planCmd.SetOut(&rendered)
	if err := printDoctorDriftPlan(context.Background(), planCmd, dir); err != nil {
		t.Fatal(err)
	}
	if p == nil || rendered.Len() == 0 || doctorTreeHash(t, dir) != before {
		t.Fatal("plan was empty or wrote files")
	}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("force", false, "")
	if err := cmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	r, err = runDriftFix(context.Background(), cmd, dir)
	if err != nil || r.Status != "pass" {
		t.Fatalf("fix: %+v, %v", r, err)
	}
	after := doctorTreeHash(t, dir)
	r, err = runDriftFix(context.Background(), cmd, dir)
	if err != nil || r.Status != "pass" || doctorTreeHash(t, dir) != after {
		t.Fatalf("second fix: %+v, %v", r, err)
	}
}

func TestDoctorFreshProjectSecondFixNoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("force", false, "")
	if _, err := runDriftFix(context.Background(), cmd, dir); err != nil {
		t.Fatal(err)
	}
	first := doctorTreeHash(t, dir)
	if _, err := runDriftFix(context.Background(), cmd, dir); err != nil {
		t.Fatal(err)
	}
	if second := doctorTreeHash(t, dir); second != first {
		t.Fatal("second fix changed a fresh project")
	}
}

func TestDoctorGeneratedDriftBothModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		before := doctorTreeHash(t, dir)
		r, _, err := checkGeneratedDrift(context.Background(), dir)
		if err != nil || r.Status != "pass" {
			t.Fatalf("check: %+v, %v", r, err)
		}
		if doctorTreeHash(t, dir) != before {
			t.Fatal("drift scan wrote project files")
		}
	})
}

func TestDoctorDeepFixMessagesPrinted(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = write, write
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	printDeepDoctorChecks([]doctorCheckResult{
		{Name: "[system] manual", Status: "fail", Message: "bad (manual step: docker system prune -f)"},
		{Name: "[system] fixed", Status: "pass", Message: "good (auto-fixed)"},
		{Name: "[system] failed", Status: "fail", Message: "bad (auto-fix failed: exit 1)"},
	})
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"manual step: docker system prune -f", "auto-fixed", "auto-fix failed: exit 1"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q in %q", want, data)
		}
	}
}

func TestReviewCleanApplyHasNoDoctorDrift(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n"), 0644); err != nil {
		t.Fatal(err)
	}
	req := doctorDriftRequest(dir)
	req.Seed = []byte("review-seed")
	if _, err := reconcile.Apply(context.Background(), req, reconcile.ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	r, p, err := checkGeneratedDrift(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("clean apply reported drift: %+v, plan=%+v", r, p)
	}
}

func TestReviewDoctorFixRespectsHandEditAndProdGate(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=prod\nBASE_DOMAIN=example.test\nSSL_MODE=none\nMINIO_ROOT_USER=prod-storage-user\nMINIO_ROOT_PASSWORD=long-unique-storage-passphrase\nHASURA_GRAPHQL_CORS_DOMAIN=https://example.test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	req := doctorDriftRequest(dir)
	req.Seed = []byte("review-seed")
	if _, err := reconcile.Apply(context.Background(), req, reconcile.ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(dir, "docker-compose.yml")
	f, err := os.OpenFile(compose, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n# human edit\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("force", false, "")
	before, err := os.ReadFile(compose)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runDriftFix(context.Background(), cmd, dir); err == nil || !strings.Contains(err.Error(), "E403") {
		t.Fatalf("missing E403 for no yes/force: %v", err)
	}
	if after, _ := os.ReadFile(compose); string(after) != string(before) {
		t.Fatal("refused fix changed compose")
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := runDriftFix(context.Background(), cmd, dir); err == nil || !strings.Contains(err.Error(), "E403") {
		t.Fatalf("missing E403 for no force: %v", err)
	}
	if after, _ := os.ReadFile(compose); string(after) != string(before) {
		t.Fatal("refused fix changed compose")
	}
}

// TestDoctorDriftVirtualPluginKeys covers D-0382: a plugin compose fragment is
// recorded under the virtual key "@plugins/<name>/docker-compose.plugin.yml".
// Doctor resolves it under the plugin directory, so a fresh install is clean,
// --fix stays clean on every rerun, a hand edit is drift and a removed plugin
// is not.
func TestDoctorDriftVirtualPluginKeys(t *testing.T) {
	dir := t.TempDir()
	plugins := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", plugins)
	const key = reconcile.PluginsPrefix + "fxplug/docker-compose.plugin.yml"
	fragment := filepath.Join(plugins, "fxplug", "docker-compose.plugin.yml")
	if err := os.MkdirAll(filepath.Dir(fragment), 0o755); err != nil {
		t.Fatal(err)
	}
	// The fixture shape of the P7-LIVE-08 cascade suite: the build rewrites an
	// nself/* image with a Dockerfile in place, so it records the fragment.
	for name, body := range map[string]string{
		"plugin.json":               `{"name":"fxplug","version":"1.0.0","description":"synthetic fixture","category":"utility","license":"MIT","port":3961,"language":"go"}`,
		"docker-compose.plugin.yml": "services:\n  fxplug:\n    image: nself/fxplug:latest\n    ports:\n      - \"3961:3961\"\n",
		"Dockerfile":                "FROM scratch\n",
	} {
		if err := os.WriteFile(filepath.Join(plugins, "fxplug", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("yes", true, "")
	cmd.Flags().Bool("force", false, "")
	// The first fix builds the fresh project with doctor's own seed, the way
	// a user's first build does; its report lists every new file.
	if _, err := runDriftFix(context.Background(), cmd, dir); err != nil {
		t.Fatal(err)
	}
	state, err := reconcile.LoadGeneratedState(dir)
	if err != nil || state == nil {
		t.Fatalf("state: %v, %v", state, err)
	}
	if _, ok := state.Files[key]; !ok {
		t.Fatalf("the build did not record %s; recorded keys: %v", key, mapKeys(state.Files))
	}

	clean := func(stage string) {
		t.Helper()
		r, _, err := checkGeneratedDrift(context.Background(), dir)
		if err != nil || r.Status != "pass" || r.Message != "no generated-file drift" {
			t.Fatalf("%s: %+v, %v", stage, r, err)
		}
	}
	clean("after install")
	for i := 1; i <= 2; i++ {
		r, err := runDriftFix(context.Background(), cmd, dir)
		if err != nil || r.Status != "pass" || r.Message != "no generated-file drift" {
			t.Fatalf("fix run %d: %+v, %v", i, r, err)
		}
		clean(fmt.Sprintf("after fix run %d", i))
	}

	f, err := os.OpenFile(fragment, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("# hand edit\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	r, _, err := checkGeneratedDrift(context.Background(), dir)
	if err != nil || r.Status != "fail" || r.Message != "drift: "+key+" (hand-edited)" {
		t.Fatalf("hand-edited fragment: %+v, %v", r, err)
	}

	if err := os.RemoveAll(filepath.Join(plugins, "fxplug")); err != nil {
		t.Fatal(err)
	}
	clean("after the plugin directory was removed")
}

func TestVirtualStateKey(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want bool
	}{
		{reconcile.PluginsPrefix + "a/docker-compose.plugin.yml", true},
		{reconcile.FrontingPrefix + "nginx/sites/a.conf", true},
		{reconcile.AbsPrefix + "etc/nself/a.conf", true},
		{"docker-compose.yml", false},
		{"nginx/plugins/a.conf", false},
		{"plugins@/a", false},
	} {
		if got := virtualStateKey(tc.key); got != tc.want {
			t.Errorf("virtualStateKey(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
