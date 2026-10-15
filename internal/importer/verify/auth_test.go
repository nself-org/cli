package verify

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/portable"
)

func TestCompareAuthFields(t *testing.T) {
	want := User{ID: "u", HashAlgorithm: "none", DefaultRole: "restricted", Roles: []string{"restricted", "reader"}, EmailVerified: true, Disabled: true, Locale: "fr", PhoneNumberVerified: true, IsAnonymous: true, Metadata: map[string]any{"access": "limited"}}
	for name, mutate := range map[string]func(*User){
		"default role":       func(u *User) { u.DefaultRole = "user" },
		"empty default role": func(u *User) { u.DefaultRole = "" },
		"roles":              func(u *User) { u.Roles = []string{"restricted", "admin"} },
		"email verified":     func(u *User) { u.EmailVerified = false },
		"disabled":           func(u *User) { u.Disabled = false },
		"metadata":           func(u *User) { u.Metadata = map[string]any{"access": "full"} },
		"locale":             func(u *User) { u.Locale = "en" },
		"empty locale":       func(u *User) { u.Locale = "" },
		"phone verified":     func(u *User) { u.PhoneNumberVerified = false },
		"anonymous":          func(u *User) { u.IsAnonymous = false },
	} {
		t.Run(name, func(t *testing.T) {
			got := want
			mutate(&got)
			state := State{Users: map[string]string{"u": "none"}, UserAuth: map[string]User{"u": got}}
			mm := CompareState(state, portable.Manifest{}, []User{want}, nil)
			if len(mm) != 1 || mm[0].Kind != "user_auth" || mm[0].ID != "u" {
				t.Fatalf("auth drift accepted: %+v", mm)
			}
			m := portable.Manifest{Exemptions: []portable.Exemption{{Kind: "user", ID: "u"}}}
			if mm := CompareState(state, m, []User{want}, nil); len(mm) != 0 {
				t.Fatalf("user exemption ignored: %+v", mm)
			}
		})
	}
}

func TestCompareAuthCanonicalAndLegacy(t *testing.T) {
	want := User{ID: "u", HashAlgorithm: "none", Roles: []string{"reader", "user"}}
	got := want
	got.DefaultRole, got.Locale = "user", "en"
	got.Roles = []string{"user", "reader"}
	got.Metadata = map[string]any{}
	state := State{Users: map[string]string{"u": "none"}, UserAuth: map[string]User{"u": got}}
	if mm := CompareState(state, portable.Manifest{}, []User{want}, nil); len(mm) != 0 {
		t.Fatalf("equivalent auth fields differ: %+v", mm)
	}
	for _, field := range []string{"default_role", "locale"} {
		empty := got
		if field == "default_role" {
			empty.DefaultRole = ""
		} else {
			empty.Locale = ""
		}
		state.UserAuth["u"] = empty
		if mm := CompareState(state, portable.Manifest{}, []User{want}, nil); len(mm) != 1 {
			t.Fatalf("empty target %s treated as a legacy source default: %+v", field, mm)
		}
	}
	delete(state.UserAuth, "u")
	if mm := CompareState(state, portable.Manifest{}, []User{want}, nil); len(mm) != 1 {
		t.Fatalf("missing observation accepted: %+v", mm)
	}
}

type authRows struct {
	pgx.Rows
	data []byte
	done bool
}

func (r *authRows) Next() bool { return !r.done }
func (r *authRows) Scan(dest ...any) error {
	*dest[0].(*[]byte) = r.data
	r.done = true
	return nil
}
func (*authRows) Err() error { return nil }

func TestObserveAuthFields(t *testing.T) {
	raw := []byte(`{"id":"u","password_hash":null,"email_verified":true,"disabled":true,"default_role":"restricted","roles":["restricted"],"metadata":{"access":"limited"},"locale":"fr","is_anonymous":true,"phone_number_verified":true}`)
	state := State{Users: map[string]string{}, UserIdent: map[string]string{}, UserAuth: map[string]User{}}
	if err := observeAuthRows(&authRows{data: raw}, &state); err != nil {
		t.Fatal(err)
	}
	var want User
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	want.HashAlgorithm = "none"
	if mm := CompareState(state, portable.Manifest{}, []User{want}, nil); len(mm) != 0 {
		t.Fatalf("observed auth fields differ: %+v", mm)
	}
	got := state.UserAuth["u"]
	if got.DefaultRole != "restricted" || !got.EmailVerified || !got.Disabled || len(got.Roles) != 1 || got.Metadata["access"] != "limited" {
		t.Fatalf("observation lost auth fields: %+v", got)
	}
}

func TestObserveAuthMetadataPrecision(t *testing.T) {
	state := State{Users: map[string]string{}, UserIdent: map[string]string{}, UserAuth: map[string]User{}}
	raw := []byte(`{"id":"u","default_role":"user","locale":"en","metadata":{"account":9007199254740993}}`)
	if err := observeAuthRows(&authRows{data: raw}, &state); err != nil {
		t.Fatal(err)
	}
	want := User{ID: "u", HashAlgorithm: "none", Metadata: map[string]any{"account": json.Number("9007199254740992")}}
	if mm := CompareState(state, portable.Manifest{}, []User{want}, nil); len(mm) != 1 {
		t.Fatalf("metadata precision loss concealed drift: %+v", mm)
	}
}
