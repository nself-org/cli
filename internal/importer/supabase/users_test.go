package supabase

import (
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/portable"
	"testing"
)

func TestNormalizeUser(t *testing.T) {
	for _, tc := range []struct {
		name, phone, want string
		anonymous         bool
	}{
		{"digits", "15551234567", "+15551234567", false},
		{"already international", "+15551234567", "+15551234567", false},
		{"empty", "", "", false},
		{"formatted", "555-1234", "555-1234", false},
		{"anonymous", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := verify.User{ID: "fixture-user", Phone: &tc.phone}
			m := portable.Manifest{}
			raw := []byte(`{"is_anonymous":false}`)
			if tc.anonymous {
				raw = []byte(`{"is_anonymous":true}`)
			}
			normalizeUser(&m)(&u, raw)
			if *u.Phone != tc.want {
				t.Fatalf("phone = %q, want %q", *u.Phone, tc.want)
			}
			if tc.anonymous && (len(m.Exemptions) != 1 || m.Exemptions[0].ID != "user:fixture-user#anonymous") {
				t.Fatalf("anonymous not reported: %+v", m.Exemptions)
			}
			if !tc.anonymous && len(m.Exemptions) != 0 {
				t.Fatal("unexpected exemption")
			}
		})
	}
}
