package verify

// auth_state_test.go — the verifier compares default role, email_verified,
// disabled and roles of each loaded user (D-0725).

import (
	"encoding/json"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

func TestCompareStateUserAuth(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	base := User{ID: id, HashAlgorithm: "bcrypt", EmailVerified: true, Roles: []string{"editor", "user"}, DefaultRole: "editor"}
	ok := UserAuth{DefaultRole: "editor", EmailVerified: true, Roles: []string{"editor", "user"}}
	with := func(f func(*UserAuth)) UserAuth {
		u := ok
		u.Roles = append([]string(nil), ok.Roles...)
		f(&u)
		return u
	}
	cases := []struct {
		name string
		want User
		got  UserAuth
		miss bool
	}{
		{"identical", base, ok, false},
		{"default role drifted to user", base, with(func(u *UserAuth) { u.DefaultRole = "user" }), true},
		{"email_verified lost", base, with(func(u *UserAuth) { u.EmailVerified = false }), true},
		{"disabled lost", func() User { b := base; b.Disabled = true; return b }(), ok, true},
		{"role missing", base, with(func(u *UserAuth) { u.Roles = []string{"user"} }), true},
		{"role added", base, with(func(u *UserAuth) { u.Roles = append(u.Roles, "admin") }), true},
		{"record without default_role reads as user", func() User { b := base; b.DefaultRole = ""; return b }(), with(func(u *UserAuth) { u.DefaultRole = "user" }), false},
		{"record without default_role, target kept editor", func() User { b := base; b.DefaultRole = ""; return b }(), ok, true},
		{"target adds its own default role", User{ID: id, HashAlgorithm: "bcrypt", Roles: []string{}}, UserAuth{DefaultRole: "user", Roles: []string{"user"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := State{Users: map[string]string{id: "bcrypt"}, UserAuth: map[string]UserAuth{id: c.got}}
			got := CompareState(state, portable.Manifest{}, []User{c.want}, nil)
			if c.miss && (len(got) != 1 || got[0].Kind != "user" || got[0].ID != id) {
				t.Fatalf("want one user mismatch, got %+v", got)
			}
			if !c.miss && len(got) != 0 {
				t.Fatalf("want none, got %+v", got)
			}
		})
	}
	t.Run("nil UserAuth skips the check", func(t *testing.T) {
		state := State{Users: map[string]string{id: "bcrypt"}}
		if got := CompareState(state, portable.Manifest{}, []User{base}, nil); len(got) != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("target user absent from the auth read", func(t *testing.T) {
		state := State{Users: map[string]string{id: "bcrypt"}, UserAuth: map[string]UserAuth{}}
		if got := CompareState(state, portable.Manifest{}, []User{base}, nil); len(got) != 1 {
			t.Fatalf("got %+v", got)
		}
	})
}

// TestUserDefaultRoleJSON: the member carries default_role, and a record
// without it still decodes.
func TestUserDefaultRoleJSON(t *testing.T) {
	var u User
	if err := json.Unmarshal([]byte(`{"id":"a","default_role":"editor"}`), &u); err != nil || u.DefaultRole != "editor" {
		t.Fatalf("default_role not decoded: %+v %v", u, err)
	}
	if err := json.Unmarshal([]byte(`{"id":"a"}`), &u); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(User{ID: "a"})
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if _, has := m["default_role"]; has {
		t.Errorf("an unset default_role is written: %s", out)
	}
}
