package firebase

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/portable"
)

const authFixture = "../../../test/fixtures/import/firebase/auth-export.json"

func collectFixture(t *testing.T, d *fakeDB) (*collector, string) {
	t.Helper()
	fs, _, _ := fakeClient(t, d)
	dir := t.TempDir()
	col := newCollector(fs, dir)
	if err := col.walk(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	if err := col.close(); err != nil {
		t.Fatal(err)
	}
	return col, dir
}

// TestMapperGolden checks the DDL and COPY files for agreeing and disagreeing fields, a nested
// map, an array, a null, an empty document and a sub-collection.
func TestMapperGolden(t *testing.T) {
	col, dir := collectFixture(t, fixtureDB())
	script, copies, notes, err := emit(col.sorted(), dir)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	out.WriteString(script)
	for _, c := range copies {
		b, _ := os.ReadFile(c.Path)
		out.WriteString("-- " + c.Table + " (" + strings.Join(c.Columns, ",") + ")\n" + string(b))
	}
	for _, n := range notes {
		out.WriteString("-- note " + n.ID + ": " + n.Reason + "\n")
	}
	golden := filepath.Join("testdata", "mapping.golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err = os.WriteFile(golden, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != string(want) {
		t.Fatalf("mapping differs from %s (UPDATE_GOLDEN=1 regenerates):\n%s", golden, out.String())
	}
	for _, tb := range col.sorted() {
		if tb.listed != tb.counted {
			t.Fatalf("%s: listing %d != count %d", tb.pattern, tb.listed, tb.counted)
		}
	}
}

func TestMapperConflictsReportedNotGuessed(t *testing.T) {
	col, dir := collectFixture(t, fixtureDB())
	_, _, notes, err := emit(col.sorted(), dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, n := range notes {
		got[n.ID] = n.Reason
	}
	for id, part := range map[string]string{
		"firebase.users.age":   "field types differ (integer,string)",
		"firebase.notes.title": "collides",
		"firebase.notes.Title": "collides",
		"firebase.notes.id":    "collides",
	} {
		if !strings.Contains(got[id], part) {
			t.Fatalf("note %s = %q, want %q (all: %v)", id, got[id], part, got)
		}
	}
}

func TestSanitizeAndFit(t *testing.T) {
	for in, want := range map[string]string{"Users": "users", "my-coll.v2": "my_coll_v2", "9lives": "_9lives", "": "_"} {
		if got := sanitize(in); got != want {
			t.Fatalf("sanitize(%q)=%q want %q", in, got, want)
		}
	}
	long := strings.Repeat("a", 80)
	if a, b := fit(long, "x"), fit(long, "y"); len(a) != 63 || a == b {
		t.Fatalf("fit must give distinct 63-byte names: %q %q", a, b)
	}
}

func TestTableNameCollisionRenamed(t *testing.T) {
	d := fixtureDB()
	d.subs[""] = []string{"Users", "users"}
	d.docs["Users"] = []fakeDoc{{ID: "a", Fields: map[string]any{}}}
	d.docs["users"] = []fakeDoc{{ID: "b", Fields: map[string]any{}}}
	d.subs["users/u1"], d.subs["users/u2"] = nil, nil
	col, dir := collectFixture(t, d)
	_, copies, notes, err := emit(col.sorted(), dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, c := range copies {
		names = append(names, c.Table)
	}
	if strings.Join(names, ",") != "Users,users" && strings.Join(names, ",") != "users,users_2" {
		t.Fatalf("tables %v", names)
	}
	if len(notes) == 0 || !strings.Contains(notes[0].Reason, "renamed") {
		t.Fatalf("collision not reported: %v", notes)
	}
}

func TestDecodeValueKinds(t *testing.T) {
	cases := map[string]kind{
		`{"stringValue":"x"}`: kString, `{"integerValue":"7"}`: kInteger, `{"doubleValue":2.5}`: kDouble,
		`{"doubleValue":"NaN"}`: kDouble, `{"booleanValue":true}`: kBoolean, `{"nullValue":null}`: kNull,
		`{"timestampValue":"2024-01-01T00:00:00Z"}`: kTimestamp, `{"bytesValue":"AAE="}`: kBytes,
		`{"referenceValue":"projects/p/databases/(default)/documents/a/b"}`: kReference,
		`{"geoPointValue":{"latitude":1,"longitude":2}}`:                    kGeoPoint,
		`{"arrayValue":{"values":[{"integerValue":"1"}]}}`:                  kArray,
		`{"mapValue":{"fields":{"a":{"booleanValue":false}}}}`:              kMap,
	}
	for raw, want := range cases {
		if _, k, err := decodeValue(json.RawMessage(raw), 0); err != nil || k != want {
			t.Fatalf("%s: %v %v", raw, k, err)
		}
	}
	for _, bad := range []string{`{"stringValue":"a\u0000b"}`, `{"integerValue":"1.5"}`, `{"mystery":1}`, `{"a":1,"b":2}`, `{"doubleValue":"oops"}`} {
		if _, _, err := decodeValue(json.RawMessage(bad), 0); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	// A 64-bit integer must keep every digit through the JSONB form.
	v, _, _ := decodeValue(json.RawMessage(`{"integerValue":"9007199254740993"}`), 0)
	if b, _ := json.Marshal(v); string(b) != "9007199254740993" {
		t.Fatalf("integer lost digits: %s", b)
	}
}

// readUsers runs the auth export reader and decodes the JSON lines it wrote.
func readUsers(t *testing.T, path string) (authResult, []verify.User, error) {
	t.Helper()
	var buf bytes.Buffer
	res, err := readAuthExport(path, &buf)
	users := []verify.User{}
	dec := json.NewDecoder(&buf)
	for dec.More() {
		var u verify.User
		if e := dec.Decode(&u); e != nil {
			t.Fatal(e)
		}
		users = append(users, u)
	}
	return res, users, err
}

func TestAuthExportMarksScryptUsersForReset(t *testing.T) {
	res, users, err := readUsers(t, authFixture)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 3 || len(users) != 3 || res.Written != 3 || len(res.ResetRequired) != 3 || res.Algorithms["unsupported"] != 3 {
		t.Fatalf("%+v", res)
	}
	for _, u := range users {
		if u.PasswordHash != nil {
			t.Fatal("password hash must never enter the bundle")
		}
		if u.ID == "" || u.Email == nil || u.HashAlgorithm != "unsupported" {
			t.Fatalf("user %+v", u)
		}
	}
	if users[0].CreatedAt != "2023-11-14T22:13:20Z" || !users[2].Disabled {
		t.Fatalf("fields %+v %+v", users[0], users[2])
	}
	_, again, _ := readUsers(t, authFixture)
	if again[0].ID != users[0].ID {
		t.Fatal("user ids must be deterministic")
	}
	fb := users[1].Metadata["firebase"].(map[string]any)
	if fb["uid"] != "fbuid-bob-0002" || fb["custom_attributes"].(map[string]any)["plan"] != "free" {
		t.Fatalf("metadata %v", fb)
	}
}

func TestAuthExportMixedUsers(t *testing.T) {
	res, users, err := readUsers(t, "testdata/auth-mixed.json")
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 4 || len(users) != 3 || res.Written != 3 || len(res.Exemptions) != 1 || res.Exemptions[0].Kind != "user" || res.Exemptions[0].Reason != "duplicate email" {
		t.Fatalf("%+v", res)
	}
	if len(res.ResetRequired) != 1 || res.Algorithms["none"] != 2 || res.Algorithms["unsupported"] != 1 {
		t.Fatalf("%+v", res)
	}
	if users[0].Email == nil || users[2].Email != nil || users[2].Phone == nil {
		t.Fatalf("nullable identity fields %+v", users)
	}
}

func TestAuthExportRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"nousers.json": `{"kind":"x"}`, "notobj.json": `[1]`, "nolocal.json": `{"users":[{"email":"a@b"}]}`, "trunc.json": `{"users":[{"localId":"a"`, "baddate.json": `{"users":[{"localId":"a","createdAt":"yesterday"}]}`, "badattrs.json": `{"users":[{"localId":"a","customAttributes":"{oops"}]}`} {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte(body), 0o600)
		if _, _, err := readUsers(t, p); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, _, err := readUsers(t, filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestResolveInputs(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	good := map[string]string{"project": "demo-nself", "auth-export": "a.json", "credentials-file": "k.json"}
	if s, err := resolve(importer.Input{Values: good, Env: env(nil)}); err != nil || s.FirestoreBase != firestoreProd || s.FirestoreEmulator {
		t.Fatalf("%+v %v", s, err)
	}
	if s, err := resolve(importer.Input{Values: map[string]string{"project": "demo-nself", "auth-export": "a.json"}, Env: env(map[string]string{"FIRESTORE_EMULATOR_HOST": "127.0.0.1:8080"})}); err != nil || s.FirestoreBase != "http://127.0.0.1:8080" {
		t.Fatalf("%+v %v", s, err)
	}
	for name, in := range map[string]importer.Input{
		"no project":        {Values: map[string]string{"auth-export": "a.json", "credentials-file": "k"}},
		"no auth export":    {Values: map[string]string{"project": "demo-nself", "credentials-file": "k"}},
		"no credentials":    {Values: map[string]string{"project": "demo-nself", "auth-export": "a"}, Env: env(nil)},
		"bad bucket":        {Values: map[string]string{"project": "demo-nself", "auth-export": "a", "credentials-file": "k", "bucket": "Bad Bucket"}},
		"bad emulator host": {Values: good, Env: env(map[string]string{"FIRESTORE_EMULATOR_HOST": "http://evil/x"})},
		"key and emulator":  {Values: good, Env: env(map[string]string{"FIRESTORE_EMULATOR_HOST": "127.0.0.1:8080"})},
	} {
		if err := CheckInput(in); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

// TestPlantedPaginationBugShowsSourceCountMismatch drops the last listing page; the COUNT
// aggregation still reports every document, so verify reports a source_count mismatch.
func TestPlantedPaginationBugShowsSourceCountMismatch(t *testing.T) {
	d := fixtureDB()
	d.dropLast = true
	col, _ := collectFixture(t, d)
	m := portable.Manifest{}
	state := verify.State{Tables: map[string]portable.Table{}, Counts: map[string]int64{}}
	for _, tb := range col.sorted() {
		tb.name = sanitize(tb.pattern[strings.LastIndex(tb.pattern, "/")+1:])
		if tb.parent != nil {
			tb.name = tb.parent.name + "__" + tb.name
		}
		id := schemaName + "." + tb.name
		m.DB.Tables = append(m.DB.Tables, portable.Table{Schema: schemaName, Name: tb.name, Rows: tb.listed, Hash: "h"})
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "table", ID: id, Count: tb.counted})
		state.Tables[id] = portable.Table{Schema: schemaName, Name: tb.name, Rows: tb.listed, Hash: "h"}
		state.Counts["table\x00"+id] = tb.listed
	}
	mismatches := verify.CompareState(state, m, nil, nil)
	found := false
	for _, mm := range mismatches {
		found = found || mm.Kind == "source_count"
	}
	if !found {
		t.Fatalf("a dropped page went unnoticed: %+v", mismatches)
	}
	// The healthy listing reports none.
	good, _ := collectFixture(t, fixtureDB())
	for _, tb := range good.sorted() {
		if tb.listed != tb.counted {
			t.Fatalf("healthy listing differs for %s", tb.pattern)
		}
	}
}

// TestLongCollisionNamesTerminate: collection ids longer than 54 bytes that clash after
// sanitising must still get distinct names (the counter is part of the hash key).
func TestLongCollisionNamesTerminate(t *testing.T) {
	long := strings.Repeat("A", 70)
	tables := []*tableAcc{{pattern: long}, {pattern: strings.ToLower(long)}, {pattern: strings.Repeat("a", 70) + "-"}}
	done := make(chan struct{})
	go func() { assignNames(tables); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("assignNames did not terminate")
	}
	seen := map[string]bool{}
	for _, tb := range tables {
		if len(tb.name) > 63 || seen[tb.name] {
			t.Fatalf("names not distinct and short: %q", tb.name)
		}
		seen[tb.name] = true
	}
}

// TestSubcollectionsUnderMissingParentAreWalked: a deleted parent document keeps its
// sub-collections; they are imported and the parent is never a row.
func TestSubcollectionsUnderMissingParentAreWalked(t *testing.T) {
	d := fixtureDB()
	d.docs["users"] = append(d.docs["users"], fakeDoc{ID: "gone", Missing: true})
	d.subs["users/gone"] = []string{"orders"}
	d.docs["users/gone/orders"] = []fakeDoc{{ID: "x1", Fields: map[string]any{"total": iv(7)}}}
	col, _ := collectFixture(t, d)
	byPattern := map[string]*tableAcc{}
	for _, tb := range col.sorted() {
		byPattern[tb.pattern] = tb
	}
	if byPattern["users"].listed != 3 || byPattern["users"].counted != 3 {
		t.Fatalf("the missing parent must not be a row: %+v", byPattern["users"])
	}
	if o := byPattern["users/*/orders"]; o.listed != 6 || o.counted != 6 {
		t.Fatalf("orders under the missing parent were dropped: listed %d counted %d", o.listed, o.counted)
	}
}
