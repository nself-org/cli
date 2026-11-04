package compose

// auth_image_v15_test.go — v1.5 always renders the current auth release
// (D-0660 part 2).
//
// Purpose: prove that a v1.5 project with no lock pin renders auth (the current
// release) whatever the key state of the caller, that a missing or malformed key
// at render time is refused with E059 (never a silent auth-v14), and that v1.4
// and lock pins behave as before.
// Inputs: minimalConfig plus the embedded image lock; keys are fake hex.
// Outputs: test failures naming mode, case and image.
// Constraints: no network, no Docker.

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

func TestAuthEntryByMode(t *testing.T) {
	compattest.Set(t, true)
	if got := AuthEntry(); got != "auth" {
		t.Errorf("v1.5 AuthEntry() = %q, want auth", got)
	}
	compattest.Set(t, false)
	if got := AuthEntry(); got != "auth-v14" {
		t.Errorf("v1.4 AuthEntry() = %q, want auth-v14", got)
	}
}

func TestAuthV15NoPinRendersCurrent(t *testing.T) {
	cur, _ := LockedRef("auth")
	defer SetProjectPins(nil)()
	captureAuthNotice(t)
	compattest.Set(t, true)
	setMode(t, PinningLock)
	if got := authImageFor(t, "0.36.0", fakeAuthKey); got != cur.String() {
		t.Errorf("v1.5 no pin = %q, want %q", got, cur.String())
	}
}

func TestAuthV15MissingKeyIsRefused(t *testing.T) {
	cur, _ := LockedRef("auth")
	for _, version := range []string{"", "0.36.0", cur.Version} {
		for _, key := range []string{"", "not-hex", " " + fakeAuthKey} {
			for _, mode := range []string{PinningLock, PinningLegacy} {
				compattest.Set(t, true)
				setMode(t, mode)
				defer SetProjectPins(nil)()
				captureAuthNotice(t)
				cfg := minimalConfig()
				cfg.Auth.Version = version
				cfg.Passthrough = map[string]string{AuthEncryptionKeyEnv: key}
				_, err := NewGenerator(cfg).buildAuthService()
				if err == nil {
					t.Fatalf("v=%q key=%q %s: rendered without a valid key", version, key, mode)
				}
				var ce *errs.CLIError
				if !errors.As(err, &ce) || ce.Code != "E059" || !strings.Contains(err.Error(), AuthEncryptionKeyEnv) {
					t.Errorf("v=%q key=%q %s: err = %v, want E059 naming %s", version, key, mode, err, AuthEncryptionKeyEnv)
				}
			}
		}
	}
}

func TestAuthV14CurrentVersionWithoutKeyStaysOnV14(t *testing.T) {
	cur, _ := LockedRef("auth")
	v14, _ := LockedRef("auth-v14")
	defer SetProjectPins(nil)()
	captureAuthNotice(t)
	compattest.Set(t, false)
	setMode(t, PinningLock)
	if got := authImageFor(t, cur.Version, ""); got != v14.String() {
		t.Errorf("v1.4 current version, no key = %q, want %q", got, v14.String())
	}
}
