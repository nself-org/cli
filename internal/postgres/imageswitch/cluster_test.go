package imageswitch

import (
	"strings"
	"testing"
)

const sampleGlobals = "-- roles\n\\restrict tok\nCREATE ROLE appro;\nALTER ROLE appro WITH LOGIN;\nCREATE ROLE postgres;\nALTER ROLE postgres WITH SUPERUSER PASSWORD 'x';\n"

func sampleCluster(t *testing.T) *clusterDoc {
	t.Helper()
	c, err := parseCluster(`{"major":16,"bootstrap":"postgres","databases":[
	{"name":"app","owner":"postgres","provider":"c","allow_connections":true,"collate":"en_US.utf8","ctype":"en_US.utf8","create":"CREATE DATABASE app WITH TEMPLATE = template0 OWNER = postgres","explicit_acl":true,"acl":["GRANT CONNECT ON DATABASE app TO appro"],
	 "settings":[["","search_path=public, foo"],["","work_mem=4MB"],["appro","timezone=UTC"]]},
	{"name":"postgres","owner":"appro","provider":"c","allow_connections":true,"collate":"C","ctype":"C","create":"CREATE DATABASE postgres WITH TEMPLATE = template0 OWNER = appro","explicit_acl":false,"acl":[],"settings":[]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestBuildRestoreFiles(t *testing.T) {
	c := sampleCluster(t)
	rf := buildRestoreFiles(sampleGlobals, c)
	if strings.Contains(rf.Globals, "CREATE ROLE postgres;") || !strings.Contains(rf.Globals, "CREATE ROLE appro;") || !strings.Contains(rf.Globals, "ALTER ROLE postgres WITH SUPERUSER") {
		t.Errorf("globals: only the bootstrap CREATE ROLE may be removed:\n%s", rf.Globals)
	}
	if len(rf.Tolerated) != 1 || !strings.Contains(rf.Tolerated[0], "CREATE ROLE postgres;") {
		t.Errorf("tolerated = %v", rf.Tolerated)
	}
	if !strings.Contains(rf.CreateDBs, "CREATE DATABASE app ") || strings.Contains(rf.CreateDBs, "CREATE DATABASE postgres") {
		t.Errorf("create (postgres exists after initdb and is restored in place): %s", rf.CreateDBs)
	}
	for _, want := range []string{
		`REVOKE ALL ON DATABASE "app" FROM PUBLIC;`, "GRANT CONNECT ON DATABASE app TO appro;",
		`ALTER DATABASE "app" SET search_path TO public, foo;`, // a list setting is not quoted
		`ALTER DATABASE "app" SET work_mem TO '4MB';`, `ALTER ROLE "appro" IN DATABASE "app" SET timezone TO 'UTC';`,
		`ALTER DATABASE postgres OWNER TO "appro";`,
	} {
		if !strings.Contains(rf.Replay, want) {
			t.Errorf("replay lacks %q:\n%s", want, rf.Replay)
		}
	}
	if strings.Contains(rf.Replay, "DROP DATABASE") {
		t.Error("postgres exists in the old cluster: it must not be dropped")
	}
	// A cluster without the postgres database drops the one initdb made.
	c2 := sampleCluster(t)
	c2.Databases = c2.Databases[:1]
	if rf := buildRestoreFiles(sampleGlobals, c2); !strings.Contains(rf.Replay, "DROP DATABASE postgres;") {
		t.Errorf("a cluster without postgres must drop it:\n%s", rf.Replay)
	}
}

func TestBuildRestoreFilesQuotedBootstrap(t *testing.T) {
	c := sampleCluster(t)
	c.Bootstrap = "Boot User"
	rf := buildRestoreFiles("CREATE ROLE \"Boot User\";\nCREATE ROLE other;\n", c)
	if strings.Contains(rf.Globals, `"Boot User"`) || !strings.Contains(rf.Globals, "CREATE ROLE other;") {
		t.Errorf("globals = %q", rf.Globals)
	}
	if quoteIdent(`a"b`) != `"a""b"` || quoteLiteral("it's") != "'it''s'" {
		t.Error("quoting")
	}
}

func TestCollationPrefersPostgres(t *testing.T) {
	if col, ct := sampleCluster(t).collation(); col != "C" || ct != "C" {
		t.Errorf("collation = %q %q (the postgres database wins)", col, ct)
	}
}

func TestClusterNamesAndParseError(t *testing.T) {
	c := sampleCluster(t)
	if got := strings.Join(c.names(), ","); got != "app,postgres" {
		t.Errorf("names = %s", got)
	}
	if _, err := parseCluster("not json"); err == nil {
		t.Error("expected a parse error")
	}
}
