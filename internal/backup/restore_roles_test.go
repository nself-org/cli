package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// Purpose: tests for the policy-role pre-create (P7-PROD-88). The docker double
// below logs every call and every psql stdin ("SQL: ...") to calls.log in
// order, prints $FAKE_SCHEMA for `pg_restore --schema-only` and lists
// $FAKE_EXISTING for the pg_roles query.

const rolesDockerScript = `#!/bin/sh
d="$FAKE_DOCKER_DIR"
printf '%s\n' "$*" >> "$d/calls.log"
[ "$1" = exec ] || exit 0
shift 3
case "$1" in
  pg_restore)
    cat > /dev/null
    case " $* " in *" --schema-only "*) cat "$FAKE_SCHEMA";; esac
    exit 0;;
  psql)
    stdin=$(cat)
    printf 'SQL: %s\n' "$stdin" >> "$d/calls.log"
    case "$stdin" in *pg_roles*) [ -n "$FAKE_EXISTING" ] && printf '%s\n' "$FAKE_EXISTING";; esac
    exit 0;;
esac
exit 0
`

const rolesFixture = "testdata/roles/policy_to_role.sql"

// rolesDocker installs the roles docker double serving schema and returns the
// log directory and an archive file to restore.
func rolesDocker(t *testing.T, schema string) (string, string) {
	t.Helper()
	dir := fakeDocker(t)
	if err := os.WriteFile(filepath.Join(dir, "bin", "docker"), []byte(rolesDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	schemaFile := filepath.Join(dir, "schema.sql")
	if err := os.WriteFile(schemaFile, []byte(schema), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_SCHEMA", schemaFile)
	archive := filepath.Join(t.TempDir(), "prod.sql")
	if err := os.WriteFile(archive, []byte("PGDMP\x01\x0e-archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, archive
}

func readFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(rolesFixture)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCollectPolicyRolesFromSchema(t *testing.T) {
	got, err := collectPolicyRoles(strings.NewReader(readFixture(t)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"mux_user"}) {
		t.Fatalf("fixture roles = %q, want [mux_user] (PUBLIC, CURRENT_USER, comments and quoted text ignored)", got)
	}
	for schema, want := range map[string][]string{
		"CREATE POLICY p ON t TO CURRENT_ROLE, SESSION_USER, public;":                          {},
		"CREATE POLICY p ON t FOR ALL USING (true);":                                           {},
		`CREATE POLICY p ON t TO "mux_user", Ops_Role;`:                                        {"mux_user", "ops_role"},
		"create policy p on t\n  to b_role,\n  a_role using (x = 'TO \"Bad\"');":               {"a_role", "b_role"},
		"CREATE FUNCTION f() AS $body$\nCREATE POLICY q ON t TO \"Not Valid\";\n$body$;":       {},
		"SELECT E'it\\'s; CREATE POLICY q ON t TO \"Bad\"';\nCREATE POLICY p ON t TO r1;":      {"r1"},
		"/* ; CREATE POLICY q ON t TO \"Bad\"; */ CREATE POLICY p ON t TO r2;\n\\restrict k\n": {"r2"},
	} {
		got, err := collectPolicyRoles(strings.NewReader(schema))
		if err != nil {
			t.Errorf("%q: %v", schema, err)
			continue
		}
		if len(want) == 0 {
			want = []string{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: roles %q, want %q", schema, got, want)
		}
	}
}

func TestRestoreIntoContainerCreatesPolicyRoles(t *testing.T) {
	c := &Container{Name: "nself-drill-0123456789abcdef", User: "postgres", DB: "drill"}
	dir, archive := rolesDocker(t, readFixture(t))
	if err := RestoreIntoContainer(context.Background(), c, archive); err != nil {
		t.Fatal(err)
	}
	calls := readFake(t, dir, "calls.log")
	schemaAt := strings.Index(calls, "pg_restore --schema-only -f -")
	createAt := strings.Index(calls, `SQL: CREATE ROLE "mux_user" NOLOGIN;`)
	restoreAt := strings.Index(calls, "pg_restore --no-owner --no-acl -U postgres -d drill")
	if schemaAt < 0 || createAt < 0 || restoreAt < 0 || !(schemaAt < createAt && createAt < restoreAt) {
		t.Fatalf("want schema read, then CREATE ROLE mux_user NOLOGIN, then pg_restore:\n%s", calls)
	}
	for _, l := range strings.Split(calls, "\n") {
		if strings.Contains(l, "CREATE ROLE") && l != `SQL: CREATE ROLE "mux_user" NOLOGIN;` {
			t.Errorf("unexpected role statement (only NOLOGIN, no password or privilege): %q", l)
		}
		if strings.Contains(strings.ToUpper(l), "GRANT") || strings.Contains(strings.ToUpper(l), "PASSWORD") {
			t.Errorf("a created role got more than NOLOGIN: %q", l)
		}
	}

	// A role that already exists is not created again.
	dir, archive = rolesDocker(t, readFixture(t))
	t.Setenv("FAKE_EXISTING", "mux_user")
	if err := RestoreIntoContainer(context.Background(), c, archive); err != nil {
		t.Fatal(err)
	}
	calls = readFake(t, dir, "calls.log")
	if !strings.Contains(calls, "pg_roles") || strings.Contains(calls, "CREATE ROLE") {
		t.Errorf("an existing role must be looked up and skipped:\n%s", calls)
	}
}

func TestRestoreRejectsInvalidRoleName(t *testing.T) {
	c := &Container{Name: "nself-drill-0123456789abcdef", User: "postgres", DB: "drill"}
	for _, role := range []string{
		`"Mux-User"`,
		`"x""; DROP ROLE postgres; --"`,
		`"MuxUser"`,
		`"` + strings.Repeat("a", 64) + `"`,
		`"9lives"`,
	} {
		schema := "CREATE POLICY p ON public.t FOR SELECT TO mux_user, " + role + " USING (true);\n"
		if _, err := collectPolicyRoles(strings.NewReader(schema)); !errors.Is(err, errs.ErrBackupRestoreFailed) {
			t.Errorf("%s: collect err = %v, want ErrBackupRestoreFailed", role, err)
		}
		dir, archive := rolesDocker(t, schema)
		err := RestoreIntoContainer(context.Background(), c, archive)
		if !errors.Is(err, errs.ErrBackupRestoreFailed) {
			t.Errorf("%s: restore err = %v, want ErrBackupRestoreFailed", role, err)
		}
		calls := readFake(t, dir, "calls.log")
		if strings.Contains(calls, "SQL:") || strings.Contains(calls, "--no-owner") {
			t.Errorf("%s: SQL ran before the name was refused:\n%s", role, calls)
		}
	}
}
