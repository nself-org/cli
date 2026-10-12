package appwrite

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/scratch"
	"github.com/nself-org/cli/internal/portable"
)

func TestAttributeMappingGolden(t *testing.T) {
	attrs := []attribute{{Key: "s", Type: "string"}, {Key: "i", Type: "integer"}, {Key: "f", Type: "double"}, {Key: "b", Type: "boolean"}, {Key: "d", Type: "datetime"}, {Key: "e", Type: "email"}, {Key: "u", Type: "url"}, {Key: "ip", Type: "ip"}, {Key: "en", Type: "enum"}, {Key: "tags", Type: "string", Array: true}, {Key: "id", Type: "string"}, {Key: "one", Type: "relationship", Related: "other", RelatedType: "oneToOne", Side: "parent"}, {Key: "many", Type: "relationship", Related: "other", RelatedType: "manyToMany"}, {Key: "pending", Type: "string", Status: "processing"}}
	cols, notes, err := mapAttributes(attrs)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cols {
		got = append(got, c.Name+" "+c.SQLType)
	}
	want := "id text PRIMARY KEY|created_at timestamptz|updated_at timestamptz|s text|i bigint|f double precision|b boolean|d timestamptz|e text|u text|ip text|en text|tags jsonb|attr_id text|one text|many jsonb"
	if strings.Join(got, "|") != want {
		t.Fatalf("mapping\n got %s\nwant %s", strings.Join(got, "|"), want)
	}
	if len(notes) != 2 || !strings.Contains(notes[0].Reason, "manyToMany") || notes[1].ID != "attribute:pending" {
		t.Fatalf("notes %v", notes)
	}
	sql, err := ddl([]*collection{{ID: "x", Schema: "main", Table: "x", Columns: cols[:4]}})
	if err != nil || sql != "CREATE SCHEMA \"main\";\nCREATE TABLE \"main\".\"x\" (\"id\" text PRIMARY KEY, \"created_at\" timestamptz, \"updated_at\" timestamptz, \"s\" text);\n" {
		t.Fatalf("ddl %q %v", sql, err)
	}
}

func TestAttributeNameCollision(t *testing.T) {
	if _, _, err := mapAttributes([]attribute{{Key: "a-b", Type: "string"}, {Key: "a_b", Type: "string"}}); err == nil {
		t.Fatal("colliding column names accepted")
	}
}

func TestUsersHashHandling(t *testing.T) {
	_, c := newFake(t, 0)
	us, err := c.exportUsers(context.Background())
	jsonl, auth := us.JSONL, us.Auth
	if err != nil || us.Total != 4 || auth.Users != 4 || us.MFA != 1 {
		t.Fatalf("users %v total=%d auth=%+v mfa=%d", err, us.Total, auth, us.MFA)
	}
	if auth.HashAlgorithms["bcrypt"] != 1 || auth.HashAlgorithms["argon2"] != 1 || auth.HashAlgorithms["scrypt"] != 1 || auth.HashAlgorithms["none"] != 1 {
		t.Fatalf("algorithms %v", auth.HashAlgorithms)
	}
	if len(auth.ResetRequired) != 2 || auth.ResetRequired[0] != userUUID("u2") || auth.ResetRequired[1] != userUUID("u3") {
		t.Fatalf("reset_required %v", auth.ResetRequired)
	}
	lines := strings.Split(strings.TrimSpace(string(jsonl)), "\n")
	var first, second map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &first)
	_ = json.Unmarshal([]byte(lines[1]), &second)
	if first["password_hash"] == nil || first["email"] != "one@example.invalid" || second["password_hash"] != nil {
		t.Fatalf("hash/email handling: %v %v", first, second)
	}
	if strings.Contains(string(jsonl), "argon2id") || strings.Contains(string(jsonl), "fakescrypt") {
		t.Fatal("non-bcrypt hash leaked into the bundle")
	}
	if userUUID("u1") != userUUID("u1") || userUUID("u1") == userUUID("u2") || len(userUUID("u1")) != 36 {
		t.Fatal("uuid mapping not stable")
	}
}

func rowsOf(t *testing.T, c *client, docs int) copied {
	t.Helper()
	cols, _, _ := mapAttributes([]attribute{{Key: "title", Type: "string"}, {Key: "views", Type: "integer"}, {Key: "author", Type: "relationship", Related: "authors", RelatedType: "manyToOne", Side: "parent"}})
	got, err := c.copyDocuments(context.Background(), "main", &collection{ID: "notes", Columns: cols}, filepath.Join(t.TempDir(), "n.csv"))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPaginationOver250Documents(t *testing.T) {
	_, c := newFake(t, 250)
	got := rowsOf(t, c, 250)
	if got.Rows != 250 || got.Total != 250 {
		t.Fatalf("rows=%d total=%d", got.Rows, got.Total)
	}
	b, _ := os.ReadFile(got.Path)
	if !strings.Contains(string(b), `"note ""7"", with comma"`) || !strings.Contains(string(b), `,"a1"`) {
		t.Fatalf("csv quoting/relationship: %.200s", b)
	}
}

// A double that drops the last page leaves rows < source total, which verify reports as a
// source_count mismatch (the count comes from `total`, not from the rows read).
func TestPlantedPaginationBugShowsCountMismatch(t *testing.T) {
	f, c := newFake(t, 250)
	f.dropLast = true
	got := rowsOf(t, c, 250)
	if got.Rows >= got.Total {
		t.Fatalf("rows=%d total=%d: planted bug not visible", got.Rows, got.Total)
	}
}

func TestWrongKeyIsRefusedWithoutEcho(t *testing.T) {
	_, c := newFake(t, 0)
	c.key = "another-fake-key"
	_, err := c.listPage(context.Background(), "/databases", "databases", "", 0)
	if err == nil || strings.Contains(err.Error(), "another-fake-key") || strings.Contains(err.Error(), fakeKey) {
		t.Fatalf("err %v", err)
	}
}

func TestEndpointValidation(t *testing.T) {
	for _, bad := range []string{"", "ftp://x", "http://user:pw@host/v1", "http://host/v1?key=1", "://", "http://appwrite.example.invalid/v1"} {
		if _, err := newClient(bad, "proj1", "k"); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	c, err := newClient("https://cloud.example.invalid/", "proj1", "k")
	if err != nil || c.base != "https://cloud.example.invalid/v1" {
		t.Fatalf("%v %v", c, err)
	}
}

func TestExtractBuildsPortableBundle(t *testing.T) {
	_, c := newFake(t, 3)
	srv := c.base
	build = func(_ context.Context, _ *config.Config, sql string, copies []scratch.CopyFile, stage string) (portable.Manifest, error) {
		if !strings.Contains(sql, `CREATE TABLE "main"."notes"`) || len(copies) != 2 {
			t.Errorf("sql/copies %q %d", sql, len(copies))
		}
		w, err := portable.NewWriter(stage)
		if err != nil {
			return portable.Manifest{}, err
		}
		for _, n := range []string{"data.dump", "schema.sql"} {
			if _, err = w.WriteFile("db/"+n, bytes.NewReader([]byte("x"))); err != nil {
				return portable.Manifest{}, err
			}
		}
		m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: "1", Source: "postgres"}, DB: portable.DB{Schemas: []string{"main"}}}
		for _, cf := range copies {
			m.DB.Tables = append(m.DB.Tables, portable.Table{Schema: cf.Schema, Name: cf.Table, Hash: "0", PK: []string{}})
		}
		return w.Finish(m)
	}
	t.Cleanup(func() { build = scratch.Build })
	stage := filepath.Join(t.TempDir(), "run", "stage")
	t.Setenv("FAKE_APPWRITE_KEY", fakeKey)
	in := importer.Input{Values: map[string]string{"endpoint": srv, "project": "proj1", "api-key-env": "FAKE_APPWRITE_KEY"}, Env: os.Getenv, Target: &config.Config{}}
	if _, err := (Source{}).Extract(context.Background(), in, stage); err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(stage)
	if err != nil {
		t.Fatal(err)
	}
	m := r.Manifest()
	if m.Producer.Source != "appwrite" || m.Auth.Users != 4 || len(m.Auth.ResetRequired) != 2 || len(m.Storage.Objects) != 2 || m.Hasura.Relationships != 1 {
		t.Fatalf("manifest %+v", m)
	}
	want := map[string]int64{"table:main.authors": 1, "table:main.notes": 3, "users:": 4, "objects:files-one": 2, "objects:": 2}
	for _, sc := range m.SourceCounts {
		if want[sc.Kind+":"+sc.ID] != sc.Count {
			t.Errorf("source count %s:%s = %d", sc.Kind, sc.ID, sc.Count)
		}
		delete(want, sc.Kind+":"+sc.ID)
	}
	if len(want) != 0 {
		t.Errorf("missing source counts %v", want)
	}
	ids := map[string]bool{}
	for _, e := range m.Exemptions {
		ids[e.ID] = true
		if !strings.Contains(e.ID, "#") {
			t.Errorf("exemption id %q lacks a #suffix and could silence a verify check", e.ID)
		}
	}
	for _, want := range []string{"main.notes#permissions", "bucket:files-one#permissions", "bucket:files-one#renamed:f2", "users#mfa"} {
		if !ids[want] {
			t.Errorf("missing exemption %s in %+v", want, m.Exemptions)
		}
	}
	if m.Storage.Objects[0].Bucket != "files-one" || !strings.HasPrefix(m.Storage.Objects[0].Key, "f1/") {
		t.Errorf("object naming %+v", m.Storage.Objects)
	}
	if err = r.Verify(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(stage), "base")); err == nil {
		t.Error("scratch base left behind")
	}
}

func TestDetectNeverEchoesKey(t *testing.T) {
	_, c := newFake(t, 0)
	t.Setenv("FAKE_APPWRITE_KEY", "wrong-fake-key")
	in := importer.Input{Values: map[string]string{"endpoint": strings.TrimSuffix(c.base, "/v1"), "project": "proj1", "api-key-env": "FAKE_APPWRITE_KEY"}, Env: os.Getenv}
	_, err := (Source{}).Detect(context.Background(), in)
	if err == nil || strings.Contains(err.Error(), "wrong-fake-key") {
		t.Fatalf("err %v", err)
	}
	t.Setenv("FAKE_APPWRITE_KEY", fakeKey)
	d, err := (Source{}).Detect(context.Background(), in)
	if err != nil || d.Kind != "appwrite" || d.Version != "1.5.10" || d.SourceUsers != 4 || len(d.Schemas) != 1 {
		t.Fatalf("detect %+v %v", d, err)
	}
}

func TestListTotalCapFallsBackToRowsRead(t *testing.T) {
	counts, ex := addCount(nil, nil, "table", "main.big", "documents", totalCap, 12000)
	if counts[0].Count != 12000 || len(ex) != 1 || ex[0].ID != "table:main.big#count" {
		t.Fatalf("capped %+v %+v", counts, ex)
	}
	// Below the cap, or with a lost page, the server total stays authoritative.
	counts, ex = addCount(nil, nil, "table", "main.small", "documents", 250, 240)
	if counts[0].Count != 250 || len(ex) != 0 {
		t.Fatalf("uncapped %+v %+v", counts, ex)
	}
	counts, _ = addCount(nil, nil, "table", "main.lost", "documents", totalCap, 4900)
	if counts[0].Count != totalCap {
		t.Fatalf("lost page at the cap hidden: %+v", counts)
	}
}

func TestNameMapping(t *testing.T) {
	for in, want := range map[string]string{"main": "main", "auth": "aw_auth", "np_x": "aw_np_x", "1db": "_1db", "my-db.v2": "my_db_v2"} {
		if got, err := schemaName(in); err != nil || got != want {
			t.Errorf("schemaName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for in, want := range map[string]string{"uploads_": "uploads-0", "a": "a00", "Files_One": "files-one"} {
		if got, err := bucketName(in); err != nil || got != want {
			t.Errorf("bucketName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestTableAndHashEdgeCases(t *testing.T) {
	if got, err := tableName("np_audit"); err != nil || got != "aw_np_audit" {
		t.Fatalf("tableName = %q, %v", got, err)
	}
	// A hash tagged bcrypt that is not bcrypt must be listed for a reset, not claimed as bcrypt.
	u, err := convertUser(apiUser{ID: "x", Registration: "2026-01-01T00:00:00.000+00:00", Password: "not-a-bcrypt-string", Hash: "bcrypt"})
	if err != nil || u.HashAlgorithm != "unsupported" || u.PasswordHash != nil {
		t.Fatalf("malformed bcrypt: %+v %v", u, err)
	}
}

func TestMissingAttributeKeyIsCounted(t *testing.T) {
	f, c := newFake(t, 7)
	f.omit = "author"
	got := rowsOf(t, c, 7)
	if got.Rows != 7 || got.Missing["author"] != 7 || got.Missing["title"] != 0 {
		t.Fatalf("missing %v rows %d", got.Missing, got.Rows)
	}
}

func TestRepeatedPageEndsWithError(t *testing.T) {
	f, c := newFake(t, 250)
	f.repeat = true
	cols, _, _ := mapAttributes([]attribute{{Key: "title", Type: "string"}})
	_, err := c.copyDocuments(context.Background(), "main", &collection{ID: "notes", Columns: cols}, filepath.Join(t.TempDir(), "n.csv"))
	if err == nil {
		t.Fatal("a server that repeats page one must not loop forever or succeed")
	}
}
