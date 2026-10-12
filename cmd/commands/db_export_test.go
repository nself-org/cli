package commands

import (
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/exporter"
)

const (
	fakeExportPostgres = "FAKE-EXPORT-POSTGRES-PASSWORD-0000000000"
	fakeExportAdmin    = "FAKE-EXPORT-HASURA-ADMIN-SECRET-0000000000000"
)

// restoreProcessEnv snapshots the whole process environment and restores it
// exactly on cleanup. config.Load exports the project's .env into the process,
// and later tests must not inherit those fixture values.
func restoreProcessEnv(t *testing.T) {
	t.Helper()
	saved := os.Environ()
	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range saved {
			k, v, _ := strings.Cut(kv, "=")
			_ = os.Setenv(k, v)
		}
	})
}

// dbExportFixture prepares the export command against a fake project and
// returns the command and the project directory.
func dbExportFixture(t *testing.T) (project string) {
	t.Helper()
	restoreProcessEnv(t)
	project = makeNSelfProject(t, "PROJECT_NAME=e2e-adopt-export\nPOSTGRES_PASSWORD="+fakeExportPostgres+"\nHASURA_GRAPHQL_ADMIN_SECRET="+fakeExportAdmin+"\n")
	t.Chdir(project)
	old := exportProject
	t.Cleanup(func() { exportProject = old })
	dbExportCmd.SetContext(context.Background())
	dbExportCmd.SetOut(io.Discard)
	if err := dbExportCmd.Flags().Set("out", "bundle"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbExportCmd.Flags().Set("out", "") })
	return project
}

func TestDBExportCommand(t *testing.T) {
	if dbExportCmd.Parent() != dbCmd {
		t.Fatal("not under db")
	}
	for _, flag := range []string{"out", "schemas", "no-storage", "json"} {
		if dbExportCmd.Flags().Lookup(flag) == nil {
			t.Fatal("missing", flag)
		}
	}
	project := dbExportFixture(t)
	// A name that only the host shell defines must never reach keys.txt.
	t.Setenv("HOST_SHELL_ONLY_FAKE_KEY", "FAKE-HOST-VALUE-0000000000")
	called := false
	exportProject = func(_ context.Context, cfg *config.Config, opts exporter.Options) (exporter.Summary, error) {
		called = true
		if cfg.ProjectName != "e2e-adopt-export" || opts.Out != "bundle" || opts.ProjectDir != project {
			t.Fatal("bad export request")
		}
		for _, want := range []string{fakeExportPostgres, fakeExportAdmin} {
			if !exportHas(opts.SecretValues, want) {
				t.Fatal("scrubber is missing a project secret value")
			}
		}
		for _, name := range []string{"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", "PROJECT_NAME"} {
			if !exportHas(opts.Keys, name) {
				t.Fatal("keys.txt is missing", name)
			}
		}
		for _, k := range opts.Keys {
			if strings.Contains(k, "FAKE-") || k == "HOST_SHELL_ONLY_FAKE_KEY" || strings.Contains(k, "=") {
				t.Fatal("keys hold a value or a host-only name", k)
			}
		}
		return exporter.Summary{Out: "bundle"}, nil
	}
	if err := runDBExport(dbExportCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("export not executed")
	}
}

func exportHas(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestDBExportProjectKeysIgnoreShell(t *testing.T) {
	restoreProcessEnv(t)
	dir := makeNSelfProject(t, "PROJECT_NAME=keys-fixture\nZED_FAKE=FAKE-VALUE-0000000000\nALPHA_FAKE=x\n")
	t.Setenv("HOST_SHELL_ONLY_FAKE_KEY", "FAKE-HOST-VALUE-0000000000")
	first := dbExportProjectKeys(dir)
	t.Setenv("ANOTHER_SHELL_ONLY_FAKE_KEY", "FAKE-OTHER-0000000000")
	if second := dbExportProjectKeys(dir); !reflect.DeepEqual(first, second) {
		t.Fatalf("keys depend on the shell: %v vs %v", first, second)
	}
	if !reflect.DeepEqual(first, []string{"ALPHA_FAKE", "PROJECT_NAME", "ZED_FAKE"}) {
		t.Fatalf("unexpected keys %v", first)
	}
}

func TestDBExportSecretValues(t *testing.T) {
	values := dbExportSecretValues([]string{"AUTH_SMTP_PASS=FAKE-SMTP-PASS", "MY_CREDENTIAL=FAKE-CREDENTIAL", "PRIVATE_DATA=FAKE-PRIVATE", "ORDINARY_URL=postgres://fake:FAKE-URL-PASSWORD@localhost/test", "NORMAL=harmless", "API_KEY=short", "BADPAIR"})
	want := []string{"FAKE-CREDENTIAL", "FAKE-PRIVATE", "FAKE-SMTP-PASS", "FAKE-URL-PASSWORD"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("secret class mismatch %v", values)
	}
}
