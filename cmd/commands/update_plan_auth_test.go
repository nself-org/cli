package commands

// update_plan_auth_test.go — the auth row of `nself update --plan` follows
// compose.AuthEntry (D-0661, P7-LIVE-20, D-0660): v1.4 stays on auth-v14, v1.5
// moves to the current release with a one-way note (the build generates the key,
// so it no longer gates the move), a migrated pin is never moved back, and a
// digest-only pin the lock cannot name is left as is. Keys are fake hex.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/upgrade/plan"
)

var fakeUpdateAuthKey = strings.Repeat("0f", 32)

func TestUpdatePlanAuthRow(t *testing.T) {
	v14, ok1 := compose.LockedRef("auth-v14")
	cur, ok2 := compose.LockedRef("auth")
	if !ok1 || !ok2 {
		t.Fatal("image lock needs auth and auth-v14")
	}
	migrated := "docker.io/nhost/auth:0.45.0@sha256:" + strings.Repeat("ab", 32)
	for _, tc := range []struct {
		name       string
		v15        bool
		pin        string
		want, note string // want "" = no value; reason must then contain the note text
	}{
		{"v1.4 keeps auth-v14", false, v14.String(), v14.String(), ""},
		{"v1.5 moves with a one-way note", true, v14.String(), cur.String(), "one way"},
		{"old spelling moves to the new name only", false, "docker.io/nhost/hasura-auth:0.36.0@" + v14.IndexDigest, v14.String(), ""},
		{"digest-only auth-v14 pin", false, "docker.io/nhost/auth@" + v14.IndexDigest, v14.String(), ""},
		{"v1.5 keeps the current pin", true, cur.String(), cur.String(), ""},
		{"v1.5 keeps a digest-only current pin", true, "docker.io/nhost/auth@" + cur.IndexDigest, cur.String(), ""},
		{"v1.4 never moves a migrated pin back", false, cur.String(), "", "backup restored"},
		{"v1.5 moves an older migrated pin to the current release", true, migrated, cur.String(), ""},
		{"unknown digest-only pin is left as is", false, "docker.io/nhost/auth@sha256:" + strings.Repeat("cd", 32), "", "does not know"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			c := authCandidate(tc.pin)
			if tc.want == "" {
				if c.Value != "" || !strings.Contains(c.Reason, tc.note) {
					t.Fatalf("candidate = %+v, want no value and a reason containing %q", c, tc.note)
				}
				return
			}
			if c.Value != tc.want || c.Reason != "" {
				t.Fatalf("candidate = %+v, want %s and no reason", c, tc.want)
			}
			if (tc.note == "") != (c.Note == "") || !strings.Contains(c.Note, tc.note) {
				t.Fatalf("note = %q, want containing %q", c.Note, tc.note)
			}
		})
	}
}

// Through gatherPlan: the key no longer gates the move (the build generates it),
// and the plan never prints it.
func TestUpdatePlanAuthFromConfig(t *testing.T) {
	v14, _ := compose.LockedRef("auth-v14")
	cur, _ := compose.LockedRef("auth")
	for _, tc := range []struct {
		name, pin, env string
		want           string // the pin itself = no move
	}{
		{"key in .env moves auth-v14", v14.String(), compose.AuthEncryptionKeyEnv + "=" + fakeUpdateAuthKey + "\n", cur.String()},
		{"no key still moves auth-v14", v14.String(), "", cur.String()},
		{"no key keeps the current pin", cur.String(), "", cur.String()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := updateProject(t)
			compattest.Set(t, true) // after updateProject, which pins v1.4
			t.Setenv(compose.AuthEncryptionKeyEnv, "")
			_ = os.Unsetenv(compose.AuthEncryptionKeyEnv)
			if tc.env != "" {
				if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=t\n"+tc.env), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			l, err := lock.Load(".")
			if err != nil {
				t.Fatal(err)
			}
			l.Images["auth"] = tc.pin
			if err := writeLockFile(dir, *l); err != nil {
				t.Fatal(err)
			}
			newUpdateRecorder(t)
			p, err := gatherPlan(context.Background(), newUpdateDeps(), *l, "")
			if err != nil {
				t.Fatal(err)
			}
			var row *plan.Pin
			for i := range p.Pins {
				if p.Pins[i].Kind == plan.KindImage && p.Pins[i].Name == "auth" {
					row = &p.Pins[i]
				}
			}
			switch {
			case row == nil && tc.want == tc.pin: // an unchanged pin has no row
			case row == nil:
				t.Fatal("no auth row in the plan")
			case tc.want == "" && (row.Resolvable || row.To != ""):
				t.Fatalf("auth row = %+v, want unresolvable", row)
			case tc.want != "" && (!row.Resolvable || row.To != tc.want):
				t.Fatalf("auth row = %+v, want To %s", row, tc.want)
			}
			var out bytes.Buffer
			if err := plan.Render(&out, p); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), fakeUpdateAuthKey) {
				t.Fatal("the plan must never print the key")
			}
		})
	}
}

// --yes with a migrated auth pin and no key stops before any local change: no
// plugin install, no image pin move, no CLI update, and the tree is untouched.
func TestUpdateYesAuthKeyPreflight(t *testing.T) {
	cur, _ := compose.LockedRef("auth")
	for _, unpinned := range []bool{false, true} {
		t.Run(map[bool]string{false: "digest pin", true: "tag-only unpinned entry"}[unpinned], func(t *testing.T) {
			testAuthKeyPreflight(t, cur, unpinned)
		})
	}
}

func testAuthKeyPreflight(t *testing.T, cur compose.Ref, unpinned bool) {
	dir := updateProject(t)
	t.Setenv(compose.AuthEncryptionKeyEnv, "")
	_ = os.Unsetenv(compose.AuthEncryptionKeyEnv)
	l, err := lock.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	if unpinned {
		l.Unpinned["auth"] = "docker.io/nhost/auth:0.45.0"
	} else {
		l.Images["auth"] = cur.String()
	}
	if err := writeLockFile(dir, *l); err != nil {
		t.Fatal(err)
	}
	r := newUpdateRecorder(t)
	before := hashTree(t, dir)
	_, _, err = runUpdate(t, "--yes")
	if err == nil || !strings.Contains(err.Error(), compose.AuthEncryptionKeyEnv) || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("err = %v, want the missing-key preflight error", err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("--yes applied something before the preflight: %v", r.calls)
	}
	if after := hashTree(t, dir); after != before {
		t.Fatal("--yes changed the project tree")
	}
	// With the key in .env the preflight passes and the update proceeds.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=t\n"+compose.AuthEncryptionKeyEnv+"="+fakeUpdateAuthKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runUpdate(t, "--yes"); err != nil {
		t.Fatalf("--yes with the key: %v", err)
	}
	if len(r.calls) == 0 {
		t.Fatal("--yes with the key applied nothing")
	}
}
