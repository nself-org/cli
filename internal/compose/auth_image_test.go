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
	captureAuthNotice(t)
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
		{"current version with key in v1.4", cur.Version, fakeAuthKey, "", PinningLock, cur.String()},
		{"current version without key stays on auth-v14", cur.Version, "", "", PinningLock, v14.String()},
		{"current version without key in v1.5", cur.Version, "", "1", PinningLock, v14.String()},
		{"explicit other version in v1.4", "0.40.0", "", "", PinningLock, "nhost/auth:0.40.0"},
		{"explicit other version in v1.5", "0.40.0", fakeAuthKey, "1", PinningLock, "nhost/auth:0.40.0"},
		{"latest stays on the frozen line in v1.4", "latest", "", "", PinningLock, "nhost/hasura-auth:latest"},
		{"latest stays on the frozen line in v1.5", "latest", fakeAuthKey, "1", PinningLegacy, "nhost/hasura-auth:latest"},
		{"explicit newer release without key renders (with a notice)", "0.45.0", "", "", PinningLock, "nhost/auth:0.45.0"},
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
				keyNotice := captureAuthNotice(t)
				defer SetProjectPins(map[string]string{"auth": pin})()
				t.Setenv("AUTH_VERSION", "0.36.0") // what nself init writes into .env
				for _, key := range []string{"", fakeAuthKey} {
					if got := authImageFor(t, "0.36.0", key); got != pin {
						t.Errorf("key=%t: auth image = %q, want the pin %q", key != "", got, pin)
					}
				}
				if notices.Len() != 0 || keyNotice.Len() != 0 {
					t.Errorf("a kept pin printed a notice: %q %q", notices.String(), keyNotice.String())
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

// captureAuthNotice points the key notice at a buffer and re-arms it.
func captureAuthNotice(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := authNoticeW
	authNoticeW = &buf
	authNoticed.Store(false)
	t.Cleanup(func() { authNoticeW = prev; authNoticed.Store(false) })
	return &buf
}

func TestAuthKeyNoticeOnce(t *testing.T) {
	compattest.Set(t, true)
	setMode(t, PinningLock)
	defer SetProjectPins(nil)()
	notices := captureAuthNotice(t)
	authImageFor(t, "0.36.0", "")
	authImageFor(t, "0.36.0", "")
	if got := strings.Count(notices.String(), AuthEncryptionKeyEnv); got != 1 {
		t.Errorf("notice printed %d times, want once: %q", got, notices.String())
	}
	quiet := captureAuthNotice(t)
	authImageFor(t, "0.36.0", fakeAuthKey)
	compattest.Set(t, false)
	authImageFor(t, "0.36.0", "")
	if quiet.Len() != 0 {
		t.Errorf("notice printed with a key or in v1.4: %q", quiet.String())
	}
}

func TestAuthEntryExported(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		want := "auth-v14"
		if compat.V15() {
			want = "auth"
		}
		if got, missing := AuthEntry(map[string]string{AuthEncryptionKeyEnv: fakeAuthKey}); got != want || missing {
			t.Errorf("AuthEntry(with key) = %q, %t; want %q, false", got, missing, want)
		}
		// The key is forwarded verbatim, so a padded one is not valid.
		if got, _ := AuthEntry(map[string]string{AuthEncryptionKeyEnv: " " + fakeAuthKey}); got != "auth-v14" {
			t.Errorf("AuthEntry(padded key) = %q, want auth-v14", got)
		}
		if got, missing := AuthEntry(nil); got != "auth-v14" || missing != compat.V15() {
			t.Errorf("AuthEntry(nil) = %q, %t; want auth-v14, %t", got, missing, compat.V15())
		}
	})
}

func TestAuthBootstrapMapsPreRenameRef(t *testing.T) {
	v14, _ := LockedRef("auth-v14")
	for _, ref := range []string{"nhost/hasura-auth:0.36.0", "docker.io/nhost/hasura-auth:0.36.0", "nhost/auth:0.36.0"} {
		if got, ok := CLILockedRef(ref); !ok || got != v14.String() {
			t.Errorf("CLILockedRef(%q) = %q, %t; want %q (auth-v14)", ref, got, ok, v14.String())
		}
	}
	if _, ok := CLILockedRef("nhost/hasura-auth:0.40.0"); ok {
		t.Error("an unlocked hasura-auth tag must stay unpinned")
	}
}

func TestAuthCatalogShowsModeEntry(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		setMode(t, PinningLock)
		v14, _ := LockedRef("auth-v14")
		n := 0
		for _, e := range ServiceCatalog() {
			if e.Name != "auth" {
				continue
			}
			n++
			if e.DefaultImage != v14.String() {
				t.Errorf("catalog auth image = %q, want %q", e.DefaultImage, v14.String())
			}
		}
		if n != 1 {
			t.Errorf("catalog has %d auth entries, want 1", n)
		}
	})
}

func TestAuthCurrentReleaseNeedsKey(t *testing.T) {
	cur, _ := LockedRef("auth")
	compattest.Both(t, func(t *testing.T) {
		for _, mode := range []string{PinningLock, PinningLegacy} {
			setMode(t, mode)
			captureAuthNotice(t)
			defer SetProjectPins(map[string]string{"auth": cur.String()})() // a pin nself update moved
			for _, key := range []string{"", "not-hex", " " + fakeAuthKey} {
				cfg := minimalConfig()
				cfg.Auth.Version = "0.36.0"
				cfg.Passthrough = map[string]string{AuthEncryptionKeyEnv: key}
				_, err := NewGenerator(cfg).buildAuthService()
				if err == nil || !strings.Contains(err.Error(), AuthEncryptionKeyEnv) {
					t.Errorf("%s key=%q: err = %v, want the missing-key error", mode, key, err)
				}
				if err != nil && strings.Contains(err.Error(), fakeAuthKey) {
					t.Error("the error must never echo a key")
				}
			}
			if got := authImageFor(t, "0.36.0", fakeAuthKey); got != cur.String() {
				t.Errorf("%s with key: auth image = %q, want the pin %q", mode, got, cur.String())
			}
		}
	})
}

func TestAuthTagNeedsKey(t *testing.T) {
	for tag, want := range map[string]bool{"0.36.0": false, "0.42.4": false, "0.43.0": true, "0.52.1-beta1": true, "v0.45.2": true,
		"1.0.0": true, "0.100.0": true, "0.9.0": false, "latest": false, "0.52": false, "": false} {
		if got := authTagNeedsKey(tag); got != want {
			t.Errorf("authTagNeedsKey(%q) = %t, want %t", tag, got, want)
		}
	}
}

func TestAuthExplicitNewerTagNotice(t *testing.T) {
	setMode(t, PinningLock)
	defer SetProjectPins(nil)()
	notices := captureAuthNotice(t)
	authImageFor(t, "0.45.0", "")
	if !strings.Contains(notices.String(), "AUTH_VERSION=0.45.0") {
		t.Errorf("no notice for an explicit 0.45.0 without the key: %q", notices.String())
	}
	quiet := captureAuthNotice(t)
	authImageFor(t, "0.45.0", fakeAuthKey)
	if quiet.Len() != 0 {
		t.Errorf("notice printed with a key: %q", quiet.String())
	}
}
