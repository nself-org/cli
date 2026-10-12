package commands

import (
	"context"
	"io"
	"os"
	"path/filepath"
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

func TestDBExportSecretValuesWidened(t *testing.T) {
	jwt := `{"type":"HS256","key":"FAKE-JWT-LEAF-VALUE","claims_namespace":"FAKE-NS-NOT-SECRET","tags":{"api_token":["FAKE-TOKEN-IN-ARRAY"]}}`
	sa := `{"client_email":"fake@example.invalid","private_key":"-----BEGIN FAKE-----\nFAKELINE\n-----END FAKE-----\n","private_key_id":"FAKE-KEY-ID-0001","project_id":"FAKE-PROJECT-ID"}`
	values := dbExportSecretValues([]string{
		"ALERT_URL=https://fake.example.invalid/hook?id=1&access_token=FAKE-QUERY-TOKEN&api_key=%46AKE%2BEN%20Q",
		"SIGNED_URL=https://fake.example.invalid/o?X-Amz-Signature=FAKE-SIGNATURE-0001;plain=notasecret",
		"REPO_URL=https://FAKE-GITHUB-TOKEN-0000000000@github.example.invalid/org/repo",
		"DB_URL=postgres://postgres@localhost/db",
		"SHORT_USER_URL=https://admin@fake.example.invalid/x",
		"HASURA_GRAPHQL_JWT_SECRET=" + jwt,
		"SOME_JSON_BLOB=" + `{"password":"FAKE-BLOB-PASSWORD"}`,
		"SERVICE_ACCOUNT_JSON=" + sa,
		"DOC_URL=https://fake.example.invalid/page?id=123456&mode=readonly",
	})
	for _, want := range []string{"FAKE-QUERY-TOKEN", "FAKE-SIGNATURE-0001", "FAKE-GITHUB-TOKEN-0000000000", "FAKE-JWT-LEAF-VALUE", "FAKE-BLOB-PASSWORD", "FAKE-TOKEN-IN-ARRAY", "FAKE-KEY-ID-0001", jwt, "-----BEGIN FAKE-----\nFAKELINE\n-----END FAKE-----\n"} {
		if !containsString(values, want) {
			t.Errorf("missing %q in %v", want, values)
		}
	}
	for _, unwanted := range []string{"postgres", "admin", "FAKE-NS-NOT-SECRET", "FAKE-PROJECT-ID", "notasecret", "readonly", "123456"} {
		if containsString(values, unwanted) {
			t.Errorf("ordinary value %q treated as secret", unwanted)
		}
	}
	// The percent-decoded form of a query secret is collected next to the raw one.
	if !containsString(values, "FAKE+EN Q") || !containsString(values, "%46AKE%2BEN%20Q") {
		t.Fatalf("query secret forms missing: %v", values)
	}
}

func TestDBExportSecretValuesUsernameWithPassword(t *testing.T) {
	const token = "FAKEGHTOKEN0123456789abcdefghijklmnopqrs" // fake 40-character token in the username slot
	values := dbExportSecretValues([]string{
		"GIT_REMOTE=https://" + token + ":x-oauth-basic@github.example.invalid/org/repo",
		"DB_URL=postgres://reporting:FAKE-DB-PASSWORD@localhost/db",
	})
	if len(token) != 40 || !containsString(values, token) || !containsString(values, "FAKE-DB-PASSWORD") {
		t.Fatalf("username token missing from %v", values)
	}
	if containsString(values, "reporting") {
		t.Fatalf("short username treated as secret: %v", values)
	}
}

func TestDBExportComposeEnvJoinsValues(t *testing.T) {
	dir := t.TempDir()
	if got := dbExportComposeEnv(dir); got != nil {
		t.Fatalf("absent compose.env gave %v", got)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".nself", "compose.env"), []byte("AUTH_SMTP_PASS=FAKE-STALE-RUNNING-PASS\nPLAIN=harmless\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values := dbExportSecretValues(dbExportComposeEnv(dir))
	if !reflect.DeepEqual(values, []string{"FAKE-STALE-RUNNING-PASS"}) {
		t.Fatalf("compose.env values %v", values)
	}
	if err := os.WriteFile(filepath.Join(dir, ".nself", "compose.env"), []byte("=\x00broken\n\"unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := dbExportComposeEnv(dir); got != nil {
		t.Fatalf("unparsable compose.env gave %v", got)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
