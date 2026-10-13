package imageswitch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeSQLDropsRunVaryingLines(t *testing.T) {
	a := "--\n-- Dumped by pg_dump version 16.4\n--\n\\restrict aaa\nSET statement_timeout = 0;\nSELECT pg_catalog.set_config('search_path', '', false);\n\nCREATE TABLE t (a int);  \n\\unrestrict aaa\n"
	b := "-- other comment\n\\restrict bbb\nSET client_encoding = 'UTF8';\nCREATE TABLE t (a int);\n\\unrestrict bbb\n"
	if n := firstDiff(normalizeSQL(a), normalizeSQL(b)); n != 0 {
		t.Errorf("equal schemas differ at %d", n)
	}
	c := "CREATE TABLE t (a bigint);\n"
	if firstDiff(normalizeSQL(a), normalizeSQL(c)) == 0 {
		t.Error("a changed column type must differ")
	}
	if firstDiff([]string{"x"}, []string{"x", "y"}) != 2 {
		t.Error("a longer file differs at the extra line")
	}
}

func TestCompareSetsNamesDifferencesWithoutValues(t *testing.T) {
	f, s := newFake(t)
	if _, err := s.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatal(err)
	}
	// A stage built by hand: the dump-time set "old" against a changed set "new".
	s.st.Databases = []string{"app"}
	stage := t.TempDir()
	s.stage = stage
	f.dbs = []string{"app"}
	f.writeSet(stage, "old", "")
	f.writeSet(stage, "new", "restore")
	diffs, err := s.compareSets("old", "new")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(diffs, "; ")
	if !strings.Contains(joined, "app: table public.t has the same row count but different content") {
		t.Errorf("diffs = %v", diffs)
	}
	if strings.Contains(joined, fakeHash) || strings.Contains(joined, "SCRAM") {
		t.Errorf("a difference must not carry password material: %v", diffs)
	}
}

func TestDescribeCapDiff(t *testing.T) {
	old := `{"tables":[{"table":"public.a","count":"1","hash":"1"},{"table":"public.b","count":"2","hash":"2"}],"sequences":[["public","s",1]],"extensions":[["x","1"]]}`
	nw := `{"tables":[{"table":"public.a","count":"3","hash":"1"},{"table":"public.c","count":"1","hash":"1"}],"sequences":[["public","s",9]],"extensions":[["x","2"]]}`
	got := strings.Join(describeCapDiff("db", old, nw), "|")
	for _, want := range []string{"public.a has 3 rows, expected 1", "public.b is missing", "public.c is new", "sequence values differ", "extensions or their versions differ"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if len(describeCapDiff("db", "x", "y")) != 1 {
		t.Error("an unreadable capture is one difference")
	}
}

func TestMismatchCapsTheList(t *testing.T) {
	var diffs []string
	for i := 0; i < 20; i++ {
		diffs = append(diffs, "d"+itoa(i))
	}
	err := mismatch("what", diffs)
	if !strings.Contains(err.Error(), "and 8 more") || strings.Contains(err.Error(), "d19") {
		t.Errorf("err = %v", err)
	}
}

func TestNonEmptyAndDblist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if nonEmpty(filepath.Join(dir, "a")) == nil || nonEmpty(filepath.Join(dir, "missing")) == nil {
		t.Error("an empty or missing file must fail")
	}
	if dblist([]string{"a", "b"}) != "a\nb\n" {
		t.Error("dblist")
	}
}

func TestRedactTail(t *testing.T) {
	md := "md5" + strings.Repeat("a", 32)
	in := "line1\nALTER ROLE x WITH PASSWORD 'secret-hash'\nbad SCRAM-SHA-256$4096:abc$def:ghi\n " + md + " leaked\nfailed postgresql://u:uri-secret@h/db host=h password=conn-secret\n"
	out := redactTail(in, 4)
	for _, bad := range []string{"secret-hash", "SCRAM-SHA-256$", md, "uri-secret", "conn-secret"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q still in %q", bad, out)
		}
	}
	if strings.Contains(out, "line1") || !strings.Contains(out, "<redacted>") {
		t.Errorf("tail = %q", out)
	}
}
