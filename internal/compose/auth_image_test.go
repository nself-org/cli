package compose

// auth_image_test.go — the auth image per compat mode (EPIC D17, P7-LIVE-20).
//
// Purpose: prove that v1.4 runs nhost/auth:0.36.0 (auth-v14, the digest
// nhost/hasura-auth:0.36.0 had), that v1.5 runs the current nhost/auth release
// only with an AUTH_ENCRYPTION_KEY, that an explicit AUTH_VERSION wins, and that
// a project lock pin of auth 0.36.0 survives v1.5, also in the pre-rename
// spelling.
// Inputs: minimalConfig plus the embedded image lock; keys are fake hex.
// Outputs: test failures naming mode, case and image.
// Constraints: no network, no Docker.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// fakeAuthKey is an obviously synthetic 64-hex AUTH_ENCRYPTION_KEY.
var fakeAuthKey = strings.Repeat("0f", 32)

// hasuraAuthDigest is the index digest of nhost/hasura-auth:0.36.0 the lock
// carried before the rename (images.lock.json at cli 828748ba).
const hasuraAuthDigest = "sha256:2e0713ef06e9c3c0af086c81289e9efbd87e4fa5dcda26891e5977b1e096caaf"

func authImageFor(t *testing.T, version, key string) string {
	t.Helper()
	cfg := minimalConfig()
	cfg.Auth.Version = version
	cfg.Passthrough = map[string]string{}
	if key != "" {
		cfg.Passthrough[AuthEncryptionKeyEnv] = key
	}
	svc, err := NewGenerator(cfg).buildAuthService()
	if err != nil {
		t.Fatalf("buildAuthService: %v", err)
	}
	return svc.Image
}

func TestAuthImageByMode(t *testing.T) {
	v14, ok1 := LockedRef("auth-v14")
	cur, ok2 := LockedRef("auth")
	if !ok1 || !ok2 {
		t.Fatal("the image lock lacks auth or auth-v14")
	}
	if v14.IndexDigest != hasuraAuthDigest || v14.Repository != "docker.io/nhost/auth" || v14.Version != "0.36.0" {
		t.Fatalf("auth-v14 = %s, want docker.io/nhost/auth:0.36.0@%s (the rename keeps the digest)", v14.String(), hasuraAuthDigest)
	}
	if cur.Repository != "docker.io/nhost/auth" || cur.Version == v14.Version {
		t.Fatalf("auth = %s, want a newer docker.io/nhost/auth release", cur.String())
	}
	restore := SetProjectPins(nil)
	defer restore()
	defer PinNoticeSink(&bytes.Buffer{})()
	cases := []struct {
		name, version, key string
		v15, mode          string // compat mode "", "1"; pinning mode
		want               string
	}{
		{"v1.4 lock default", "0.36.0", "", "", PinningLock, v14.String()},
		{"v1.4 lock blank version", "", "", "", PinningLock, v14.String()},
		{"v1.4 legacy default", "0.36.0", "", "", PinningLegacy, "nhost/auth:0.36.0"},
		{"v1.4 key ignored", "0.36.0", fakeAuthKey, "", PinningLock, v14.String()},
		{"v1.5 no key stays on auth-v14", "0.36.0", "", "1", PinningLock, v14.String()},
		{"v1.5 malformed key stays on auth-v14", "0.36.0", "not-hex", "1", PinningLock, v14.String()},
		{"v1.5 key moves to the current release", "0.36.0", fakeAuthKey, "1", PinningLock, cur.String()},
		{"v1.5 key legacy pinning", "", fakeAuthKey, "1", PinningLegacy, cur.LegacyRef},
		{"explicit current version in v1.4", cur.Version, "", "", PinningLock, cur.String()},
		{"explicit other version in v1.4", "0.40.0", "", "", PinningLock, "nhost/auth:0.40.0"},
		{"explicit other version in v1.5", "0.40.0", fakeAuthKey, "1", PinningLock, "nhost/auth:0.40.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15 == "1")
			setMode(t, tc.mode)
			if got := authImageFor(t, tc.version, tc.key); got != tc.want {
				t.Errorf("auth image = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuthLockPinKept(t *testing.T) {
	cur, _ := LockedRef("auth")
	pins := map[string]string{
		"nhost/auth spelling":        "docker.io/nhost/auth:0.36.0@" + hasuraAuthDigest,
		"nhost/hasura-auth spelling": "docker.io/nhost/hasura-auth:0.36.0@" + hasuraAuthDigest,
	}
	for name, pin := range pins {
		t.Run(name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				setMode(t, PinningLock)
				var notices bytes.Buffer
				defer PinNoticeSink(&notices)()
				defer SetProjectPins(map[string]string{"auth": pin})()
				t.Setenv("AUTH_VERSION", "0.36.0") // what nself init writes into .env
				for _, key := range []string{"", fakeAuthKey} {
					if got := authImageFor(t, "0.36.0", key); got != pin {
						t.Errorf("key=%t: auth image = %q, want the pin %q", key != "", got, pin)
					}
				}
				if notices.Len() != 0 {
					t.Errorf("a kept pin printed a notice: %q", notices.String())
				}
				// The current release's version is a CLI default too: the pin stays.
				t.Setenv("AUTH_VERSION", cur.Version)
				if got := authImageFor(t, cur.Version, fakeAuthKey); got != pin {
					t.Errorf("AUTH_VERSION=%s: auth image = %q, want the pin %q", cur.Version, got, pin)
				}
				// Any other AUTH_VERSION is explicit and outranks the pin.
				t.Setenv("AUTH_VERSION", "0.40.0")
				if got := authImageFor(t, "0.40.0", ""); got != "nhost/auth:0.40.0" {
					t.Errorf("explicit AUTH_VERSION=0.40.0: auth image = %q, want nhost/auth:0.40.0", got)
				}
			})
		})
	}
}

func TestAuthKeyNoticeOnce(t *testing.T) {
	compattest.Set(t, true)
	setMode(t, PinningLock)
	defer SetProjectPins(nil)()
	var notices bytes.Buffer
	defer PinNoticeSink(&notices)()
	authImageFor(t, "0.36.0", "")
	authImageFor(t, "0.36.0", "")
	if got := strings.Count(notices.String(), AuthEncryptionKeyEnv); got != 1 {
		t.Errorf("notice printed %d times, want once: %q", got, notices.String())
	}
	if strings.Contains(notices.String(), fakeAuthKey) {
		t.Error("the notice must never echo a key")
	}
}

func TestAuthEntryExported(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		want := "auth-v14"
		if compat.V15() {
			want = "auth"
		}
		if got := AuthEntry(map[string]string{AuthEncryptionKeyEnv: " " + fakeAuthKey + " "}); got != want {
			t.Errorf("AuthEntry(with key) = %q, want %q", got, want)
		}
		if got := AuthEntry(nil); got != "auth-v14" {
			t.Errorf("AuthEntry(nil) = %q, want auth-v14", got)
		}
	})
}
