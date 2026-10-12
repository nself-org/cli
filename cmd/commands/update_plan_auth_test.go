package commands

// update_plan_auth_test.go — the auth row of `nself update --plan` follows
// compose.AuthEntry (D-0661, P7-LIVE-20): v1.4 and v1.5 without
// AUTH_ENCRYPTION_KEY stay on auth-v14, v1.5 with the key moves to the current
// release with a one-way note, and a migrated pin is never moved back.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
)

func TestUpdatePlanAuthRow(t *testing.T) {
	v14, ok1 := compose.LockedRef("auth-v14")
	cur, ok2 := compose.LockedRef("auth")
	if !ok1 || !ok2 {
		t.Fatal("image lock needs auth and auth-v14")
	}
	key := map[string]string{compose.AuthEncryptionKeyEnv: strings.Repeat("0f", 32)} // fake
	oldSpelling := "docker.io/nhost/hasura-auth:0.36.0@" + v14.IndexDigest
	for _, tc := range []struct {
		name        string
		v15         bool
		pin         string
		pass        map[string]string
		want, note  string // want "" = no value (a reason instead)
		wantsReason bool
	}{
		{"v1.4 keeps auth-v14", false, v14.String(), key, v14.String(), "", false},
		{"v1.5 without key keeps auth-v14", true, v14.String(), nil, v14.String(), "", false},
		{"v1.5 with key moves with a one-way note", true, v14.String(), key, cur.String(), "one way", false},
		{"old spelling moves to the new name only", false, oldSpelling, nil, v14.String(), "", false},
		{"v1.5 with key keeps the current pin", true, cur.String(), key, cur.String(), "", false},
		{"v1.4 never moves a migrated pin back", false, cur.String(), nil, "", "", true},
		{"v1.5 without key never moves a migrated pin back", true, "docker.io/nhost/auth:0.45.0@sha256:" + strings.Repeat("ab", 32), nil, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			c := authCandidate(tc.pin, tc.pass)
			if tc.wantsReason {
				if c.Value != "" || !strings.Contains(c.Reason, "backup") {
					t.Fatalf("candidate = %+v, want no value and a restore reason", c)
				}
				return
			}
			if c.Value != tc.want {
				t.Fatalf("candidate = %+v, want %s", c, tc.want)
			}
			if (tc.note == "") != (c.Note == "") || !strings.Contains(c.Note, tc.note) {
				t.Fatalf("note = %q, want containing %q", c.Note, tc.note)
			}
		})
	}
}
