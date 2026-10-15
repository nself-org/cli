package verify

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

func TestCompareStateDetectsUserAttributeDrift(t *testing.T) {
	const expected = `{"id":"u1","hash_algorithm":"none","disabled":true,"email_verified":true,"metadata":{"private":"source-value","nested":{"n":1}},"roles":["restricted","user"],"default_role":"restricted"}`
	var want User
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, from, to string }{
		{"disabled", `"disabled":true`, `"disabled":false`},
		{"email_verified", `"email_verified":true`, `"email_verified":false`},
		{"metadata", `"n":1`, `"n":2`},
		{"roles", `"restricted","user"`, `"user"`},
		{"default_role", `"default_role":"restricted"`, `"default_role":"user"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Decode the observed snapshot fixture separately from the expected bundle.
			var state State
			raw := `{"Users":{"u1":"none"},"UserAttributes":{"u1":` + strings.Replace(expected, tc.from, tc.to, 1) + `}}`
			if err := json.Unmarshal([]byte(raw), &state); err != nil {
				t.Fatal(err)
			}
			got := CompareState(state, portable.Manifest{}, []User{want}, nil)
			if len(got) != 1 || got[0].Kind != "user" || got[0].ID != "u1" {
				t.Fatalf("%s drift passed verification: %+v", tc.name, got)
			}
			if strings.Contains(got[0].Detail, "source-value") {
				t.Fatal("mismatch leaked metadata")
			}
		})
	}
}

func TestCompareStateUserAttributesNormalization(t *testing.T) {
	want := User{ID: "u1", HashAlgorithm: "none", Roles: []string{"restricted", "user"}}
	got := User{DefaultRole: "user", Roles: []string{"user", "restricted"}, Metadata: map[string]any{}}
	state := State{Users: map[string]string{"u1": "none"}, UserAttributes: map[string]User{"u1": got}}
	if mm := CompareState(state, portable.Manifest{}, []User{want}, nil); len(mm) != 0 {
		t.Fatalf("legacy default, unordered roles or empty metadata differ: %+v", mm)
	}
	delete(state.UserAttributes, "u1")
	if mm := CompareState(state, portable.Manifest{}, []User{want}, nil); len(mm) != 1 {
		t.Fatalf("missing observation passed: %+v", mm)
	}
	m := portable.Manifest{Exemptions: []portable.Exemption{{Kind: "user", ID: "u1", Reason: "fixture"}}}
	if mm := CompareState(state, m, []User{want}, nil); len(mm) != 0 {
		t.Fatalf("user exemption ignored: %+v", mm)
	}
}
