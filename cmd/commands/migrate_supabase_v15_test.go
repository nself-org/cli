package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/importer"
	spmigrate "github.com/nself-org/cli/internal/migrate/supabase"
	"github.com/spf13/pflag"
)

func resetSupabaseFlags(t *testing.T) {
	t.Helper()
	migrateSupabaseCmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

// TestMigrateSupabaseModes: v1.4 writes the three artifacts exactly as before and still
// requires --project-ref and --supabase-key; v1.5 runs importer.Run with the supabase source,
// writes no artifact and needs neither flag.
func TestMigrateSupabaseModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		resetSupabaseFlags(t)
		t.Cleanup(func() { resetSupabaseFlags(t) })
		out := t.TempDir()
		t.Chdir(out)
		oldPull, oldRun := supabasePull, importRun
		t.Cleanup(func() { supabasePull, importRun = oldPull, oldRun })
		pulled, ran := false, false
		supabasePull = func(_ context.Context, cfg spmigrate.Config) (*spmigrate.PullResult, error) {
			pulled = true
			if cfg.ProjectRef != "abcdefghijklmnopqrst" {
				t.Fatalf("project ref %q", cfg.ProjectRef)
			}
			return &spmigrate.PullResult{Tables: []spmigrate.Table{{Schema: "public", Name: "notes"}}}, nil
		}
		importRun = func(_ context.Context, _ *config.Config, s importer.Source, in importer.Input, opts importer.RunOptions) (importer.Report, error) {
			ran = true
			if s.Name() != "supabase" || in.Values["db-url-env"] != "SUPA_DB" || in.Values["api-url"] != "http://127.0.0.1:54321" || in.Values["service-key-env"] != "SUPA_KEY" || !opts.Plan {
				t.Fatalf("wrong import inputs: %+v %+v", in.Values, opts)
			}
			return importer.Report{Source: "supabase", Result: "pass"}, nil
		}
		cmd := migrateSupabaseCmd
		cmd.SetContext(context.Background())
		if err := cmd.PreRunE(cmd, nil); err != nil {
			t.Fatal(err)
		}
		if compat.V15() {
			if err := cmd.ValidateRequiredFlags(); err != nil {
				t.Fatalf("v1.5 must not require the artifact flags: %v", err)
			}
			for k, v := range map[string]string{"db-url-env": "SUPA_DB", "api-url": "http://127.0.0.1:54321", "service-key-env": "SUPA_KEY", "plan": "true"} {
				if err := cmd.Flags().Set(k, v); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := captureHasuraOutput(func() error { return cmd.RunE(cmd, nil) }); err != nil {
				t.Fatal(err)
			}
			if !ran || pulled {
				t.Fatalf("v1.5 must run the importer only (ran=%v pulled=%v)", ran, pulled)
			}
			if _, err := os.Stat(filepath.Join(out, "supabase-migration.sql")); err == nil {
				t.Fatal("v1.5 wrote an artifact")
			}
			return
		}
		err := cmd.ValidateRequiredFlags()
		if err == nil || !strings.Contains(err.Error(), "project-ref") || !strings.Contains(err.Error(), "supabase-key") {
			t.Fatalf("v1.4 must require both flags, got %v", err)
		}
		_ = cmd.Flags().Set("project-ref", "abcdefghijklmnopqrst")
		_ = cmd.Flags().Set("supabase-key", "synthetic-key")
		_ = cmd.Flags().Set("out", out)
		if _, err = captureHasuraOutput(func() error { return cmd.RunE(cmd, nil) }); err != nil {
			t.Fatal(err)
		}
		if !pulled || ran {
			t.Fatalf("v1.4 must run the artifact generator only (pulled=%v ran=%v)", pulled, ran)
		}
		for _, name := range []string{"supabase-migration.sql", "hasura-metadata.yaml", "import-auth-users.sh"} {
			if _, err = os.Stat(filepath.Join(out, name)); err != nil {
				t.Fatalf("v1.4 artifact %s missing: %v", name, err)
			}
		}
	})
}
