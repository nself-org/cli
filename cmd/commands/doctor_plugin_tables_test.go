package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

func checkedDoctorTables(t *testing.T, cmd *cobra.Command, cwd string, fix bool) []doctorCheckResult {
	t.Helper()
	results, err := checkPluginTables(context.Background(), cmd, cwd, "", fix, false)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

func TestDoctorFixDeferredPluginTables(t *testing.T) {
	compattest.Set(t, true)
	dir := readinessProject(t, "fine", 1, 1)
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	cwd, _ := os.Getwd()
	var plugins []database.PluginTables
	calls, exports := realTracking(t, func(p []database.PluginTables) error { plugins = p; return nil }, fineMeta, nil)
	export := bundleExportMetadata
	bundleExportMetadata = func(ctx context.Context, cfg *config.Config) ([]byte, error) {
		if *calls == 0 {
			*exports++
			return []byte(`{"sources":[]}`), nil
		}
		return export(ctx, cfg)
	}
	cmd, _ := readinessCmd()
	results := checkedDoctorTables(t, cmd, cwd, true)
	if len(results) != 1 || results[0].Status != "pass" || *calls != 1 || *exports != 2 || len(plugins) != 1 || plugins[0].Plugin != "fine" {
		t.Fatalf("results=%+v calls=%d exports=%d plugins=%+v", results, *calls, *exports, plugins)
	}
	// A successful tracker response is insufficient if metadata still lacks the table.
	realTracking(t, nil, `{"sources":[]}`, nil)
	results = checkedDoctorTables(t, cmd, cwd, true)
	if len(results) != 1 || results[0].Status != "fail" || !strings.Contains(results[0].Message, "public.np_fine_items") {
		t.Fatalf("unverified repair: %+v", results)
	}
}

func TestDoctorDoesNotTrackBeforeMigrationsReady(t *testing.T) {
	compattest.Set(t, true)
	dir := readinessProject(t, "stuck", 0, 1)
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	cwd, _ := os.Getwd()
	calls, exports := realTracking(t, nil, `{"sources":[]}`, nil)
	cmd, _ := readinessCmd()
	results := checkedDoctorTables(t, cmd, cwd, true)
	if len(results) != 1 || results[0].Status != "fail" || !strings.Contains(results[0].Message, "E118") || *calls != 0 || *exports != 1 {
		t.Fatalf("results=%+v calls=%d exports=%d", results, *calls, *exports)
	}
}

func TestDoctorPluginTablesReadFailureAndLegacy(t *testing.T) {
	dir := readinessProject(t, "fine", 1, 1)
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	cwd, _ := os.Getwd()
	calls, exports := realTracking(t, nil, "", errors.New("metadata API returned 403"))
	compattest.Set(t, true)
	cmd, _ := readinessCmd()
	results := checkedDoctorTables(t, cmd, cwd, false)
	if len(results) != 1 || results[0].Status != "fail" || !strings.Contains(results[0].Message, "403") || *calls != 0 {
		t.Fatalf("metadata error hidden: %+v", results)
	}
	compattest.Set(t, false)
	before := *exports
	if got := checkedDoctorTables(t, cmd, cwd, true); len(got) != 0 || *exports != before || *calls != 0 {
		t.Fatalf("legacy mode changed: %+v", got)
	}
}

func TestDoctorPluginTablesSkipsBinaryDirectoryAndTrackedPlugins(t *testing.T) {
	compattest.Set(t, true)
	dir := readinessProject(t, "fine", 0, 1) // not ready, but no write is needed
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	calls, _ := realTracking(t, nil, fineMeta, nil)
	cmd, _ := readinessCmd()
	got := checkedDoctorTables(t, cmd, cwd, true)
	if len(got) != 1 || got[0].Status != "pass" || *calls != 0 {
		t.Fatalf("binary directory or already-tracked plugin caused repair: %+v calls=%d", got, *calls)
	}
}

func TestDoctorPluginTablesRequiresProdConfirmationWithCleanGeneratedPlan(t *testing.T) {
	compattest.Set(t, true)
	dir := readinessProject(t, "fine", 1, 1)
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	t.Setenv("ENV", "prod")
	cwd, _ := os.Getwd()
	// A settled file plan has no confirmation gate; metadata still requires one.
	p := reconcile.Plan{Env: "prod"}
	p.Containers.Known = true
	if err := p.Finalize(); err != nil || !p.Empty || p.RequiresConfirmation {
		t.Fatalf("expected clean file plan, got %+v, %v", p, err)
	}
	calls, _ := realTracking(t, nil, `{"sources":[]}`, nil)
	cmd, _ := readinessCmd()
	cmd.Flags().Bool("yes", false, "")
	got, err := checkPluginTables(context.Background(), cmd, cwd, "", true, false)
	if d := errs.Describe(err); d == nil || d.Code != "E403" || d.ExitCode != 4 || *calls != 0 {
		t.Fatalf("unconfirmed metadata change: %+v err=%v calls=%d", got, err, *calls)
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	checkedDoctorTables(t, cmd, cwd, true)
	if *calls != 1 {
		t.Fatalf("confirmed repair did not call tracker: %d", *calls)
	}
}

func TestDoctorPluginTablesConfirmsSelectorAndLoadedEnv(t *testing.T) {
	for _, pair := range [][2]string{{"prod", "dev"}, {"dev", "prod"}, {"staging", "dev"}} {
		t.Run(pair[0]+"-"+pair[1], func(t *testing.T) {
			compattest.Set(t, true)
			dir := readinessProject(t, "fine", 1, 1)
			t.Setenv("NSELF_PLUGIN_DIR", dir)
			cwd, _ := os.Getwd()
			if err := os.WriteFile(filepath.Join(cwd, ".env"), []byte("PROJECT_NAME=rdy\nENV="+pair[1]+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ENV", pair[0])
			calls, _ := realTracking(t, nil, `{"sources":[]}`, nil)
			cmd, _ := readinessCmd()
			_, err := checkPluginTables(context.Background(), cmd, cwd, "", true, false)
			if d := errs.Describe(err); d == nil || d.Code != "E403" || d.ExitCode != 4 || *calls != 0 {
				t.Fatalf("selector=%s loaded=%s: err=%v calls=%d", pair[0], pair[1], err, *calls)
			}
		})
	}
}

func TestDoctorDeepIncludesPluginTrackingAndHonorsOnly(t *testing.T) {
	compattest.Set(t, true)
	dir := readinessProject(t, "fine", 1, 1)
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	cwd, _ := os.Getwd()
	_, exports := realTracking(t, nil, `{"sources":[]}`, nil)
	cmd, _ := readinessCmd()
	got, err := doctorPluginTableDeepChecks(context.Background(), cmd, cwd, "dev", "plugins", false, nil)
	if err != nil || len(got) != 1 || got[0].Section != "plugins" || got[0].Status != "fail" || !strings.Contains(got[0].Message, "public.np_fine_items") {
		t.Fatalf("deep tracking findings=%+v err=%v", got, err)
	}
	before := *exports
	got, err = doctorPluginTableDeepChecks(context.Background(), cmd, cwd, "dev", "docker", true, nil)
	if err != nil || len(got) != 0 || *exports != before {
		t.Fatalf("section filter touched plugin tracking: %+v %v", got, err)
	}
	compattest.Set(t, false)
	beforeTree := doctorTreeHash(t, cwd)
	got, err = doctorPluginTableDeepChecks(context.Background(), cmd, cwd, "dev", "", true, nil)
	if err != nil || len(got) != 0 || *exports != before || doctorTreeHash(t, cwd) != beforeTree {
		t.Fatalf("legacy deep fix changed files or metadata: %+v %v", got, err)
	}
}
