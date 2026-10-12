package verify

import (
	"github.com/nself-org/cli/internal/portable"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleUsersAndMetadata(t *testing.T) {
	w, err := portable.NewWriter(filepath.Join(t.TempDir(), "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.WriteFile("auth/users.jsonl", strings.NewReader(`{"id":"u1","hash_algorithm":"argon2"}`+"\n")); err != nil {
		t.Fatal(err)
	}
	if _, err = w.WriteFile("hasura/metadata.json", strings.NewReader(`{"sources":[{"name":"default","tables":[{"table":{"schema":"public","name":"notes"},"select_permissions":[]}]}]}`)); err != nil {
		t.Fatal(err)
	}
	m := portable.Manifest{DB: portable.DB{Tables: []portable.Table{{Schema: "public", Name: "notes", Hash: "0"}}}, Auth: portable.Auth{Users: 1, ResetRequired: []string{"u1"}}}
	if _, err = w.Finish(m); err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(w.Dir())
	if err != nil {
		t.Fatal(err)
	}
	users, err := ReadUsers(r)
	if err != nil || len(users) != 1 {
		t.Fatalf("users: %+v %v", users, err)
	}
	meta, err := ReadMetadata(r)
	if err != nil || meta["public.notes"] == nil {
		t.Fatalf("metadata: %+v %v", meta, err)
	}
	got := CompareState(State{Tables: map[string]portable.Table{"public.notes": m.DB.Tables[0]}, Users: map[string]string{"u1": "none"}, Metadata: meta}, m, users, meta)
	if len(got) != 0 {
		t.Fatalf("reset-required algorithm: %+v", got)
	}
	for _, raw := range []string{`bad`, `{"sources":[{"name":"default","tables":[{}]}]}`} {
		if _, err := ParseMetadata([]byte(raw)); err == nil {
			t.Fatal("malformed metadata accepted")
		}
	}
}

func TestIdentityKeepsNullApart(t *testing.T) {
	empty, phone := "", "+15550001"
	if Identity(nil, &phone, nil) == Identity(&empty, &phone, nil) {
		t.Fatal("NULL email must differ from empty email")
	}
	if Identity(nil, nil, nil) != Identity(nil, nil, &empty) {
		t.Fatal("NULL display name equals empty display name")
	}
	state := State{Users: map[string]string{"a": "none", "b": "none"}, UserIdent: map[string]string{"a": Identity(nil, &phone, nil), "b": Identity(&empty, &phone, nil)}}
	users := []User{{ID: "a", HashAlgorithm: "none", Phone: &phone}, {ID: "b", HashAlgorithm: "none", Phone: &phone}}
	got := CompareState(state, portable.Manifest{}, users, nil)
	if len(got) != 1 || got[0].Kind != "user" || got[0].ID != "b" {
		t.Fatalf("want exactly the empty-vs-NULL user to mismatch: %+v", got)
	}
}
