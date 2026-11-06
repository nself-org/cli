package policyroles

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Purpose: unit tests for Collect. The restore-path tests and the fixture live
// in internal/backup (restore_roles_test.go).

func TestCollectSkipsQuotedAndCommentedText(t *testing.T) {
	schema := "-- CREATE POLICY a ON t TO ghost;\n" +
		"COMMENT ON TABLE t IS '\nCREATE POLICY b ON t TO \"Bad Name\";';\n" +
		"CREATE FUNCTION f() AS $fn$ CREATE POLICY c ON t TO \"Bad\"; $fn$;\n" +
		"CREATE POLICY \"to\" ON public.\"TO\" USING (true);\n" +
		"CREATE POLICY d ON t AS RESTRICTIVE FOR ALL TO PUBLIC, Mux_User WITH CHECK (x = 'TO y');\n"
	got, err := Collect(strings.NewReader(schema))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"mux_user"}) {
		t.Fatalf("got %q, want [mux_user]", got)
	}
}

func TestCollectRefusesNamesOutsidePattern(t *testing.T) {
	for _, role := range []string{`"Mux-User"`, `"a""b"`, `"pg admin"`, `"` + strings.Repeat("x", 64) + `"`} {
		_, err := Collect(strings.NewReader("CREATE POLICY p ON t TO " + role + ";\n"))
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("%s: err = %v, want ErrInvalidName", role, err)
		}
	}
}
