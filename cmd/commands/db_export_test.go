package commands

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/exporter"
)

func TestDBExportCommand(t *testing.T) {
	if dbExportCmd.Parent() != dbCmd {
		t.Fatal("not under db")
	}
	for _, flag := range []string{"out", "schemas", "no-storage", "json"} {
		if dbExportCmd.Flags().Lookup(flag) == nil {
			t.Fatal("missing", flag)
		}
	}
	project := makeNSelfProject(t, "PROJECT_NAME=e2e-adopt-export\nPOSTGRES_PASSWORD=FAKE-EXPORT-PASSWORD\nHASURA_GRAPHQL_ADMIN_SECRET=FAKE-EXPORT-ADMIN\n")
	t.Chdir(project)
	old := exportProject
	t.Cleanup(func() { exportProject = old })
	called := false
	exportProject = func(_ context.Context, cfg *config.Config, opts exporter.Options) (exporter.Summary, error) {
		called = true
		if cfg.ProjectName != "e2e-adopt-export" || opts.Out != "bundle" || opts.ProjectDir != project {
			t.Fatalf("bad request %+v", opts)
		}
		return exporter.Summary{Out: "bundle"}, nil
	}
	cmd := dbExportCmd
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	if err := cmd.Flags().Set("out", "bundle"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Flags().Set("out", "") })
	if err := runDBExport(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("export not executed")
	}
}

func TestDBExportSecretValues(t *testing.T) {
	values, keys := dbExportSecretValues([]string{"AUTH_SMTP_PASS=FAKE-SMTP-PASS", "MY_CREDENTIAL=FAKE-CREDENTIAL", "PRIVATE_DATA=FAKE-PRIVATE", "ORDINARY_URL=postgres://fake:FAKE-URL-PASSWORD@localhost/test", "NORMAL=harmless", "API_KEY=short", "BADPAIR"})
	want := []string{"FAKE-CREDENTIAL", "FAKE-PRIVATE", "FAKE-SMTP-PASS", "FAKE-URL-PASSWORD"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("secret class mismatch %v", values)
	}
	if len(keys) != 6 {
		t.Fatal("keys lost", keys)
	}
}
