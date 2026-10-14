package imageswitch

import (
	"os"
	"strings"
	"testing"
)

func TestNormalizeSQLPreservesFunctionSearchPath(t *testing.T) {
	for _, quote := range []string{"$$", "$function$", "'"} {
		t.Run(quote, func(t *testing.T) {
			old := "CREATE FUNCTION public.f() RETURNS void LANGUAGE plpgsql AS " + quote + "\nBEGIN\nSET LOCAL search_path = pg_catalog;\nEND\n" + quote + ";\n"
			changed := strings.Replace(old, "pg_catalog", "public", 1)
			if firstDiff(normalizeSQL(old), normalizeSQL(changed)) == 0 {
				t.Fatal("a changed function search_path must differ")
			}
		})
	}
}

func TestNormalizeSQLQuotedBodyBoundaries(t *testing.T) {
	cases := []string{
		"CREATE FUNCTION f() RETURNS text AS $body$\nSELECT '$other$';\nSET LOCAL search_path = pg_catalog;\n$body$ LANGUAGE sql;\n",
		"CREATE FUNCTION f() RETURNS text AS '\nSELECT ''$body$'';\nSET LOCAL search_path = pg_catalog;\n' LANGUAGE sql;\n",
		"-- fake $body$ delimiter\nCREATE FUNCTION f() RETURNS text AS $$\n-- body comment\nSET LOCAL search_path = pg_catalog;\n$$ LANGUAGE sql;\n",
	}
	for _, old := range cases {
		changed := strings.Replace(old, "pg_catalog", "public", 1)
		if firstDiff(normalizeSQL(old), normalizeSQL(changed)) == 0 {
			t.Errorf("a changed quoted body must differ: %q", old)
		}
		if firstDiff(normalizeSQL(old+"SET statement_timeout = 0;\n"), normalizeSQL(old+"SET statement_timeout = 1;\n")) != 0 {
			t.Errorf("session settings after the body must be ignored: %q", old)
		}
	}
}

func TestIncompleteFailsClosedOnStateSymlinkLoop(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(dir(project), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("state.json", StatePath(project)); err != nil {
		t.Fatal(err)
	}
	st, incomplete := Incomplete(project)
	if !incomplete || st == nil || st.Phase != "" {
		t.Fatalf("stat failure must refuse the switch: state=%v incomplete=%v", st, incomplete)
	}
}
