package compose

// hasura_image_test.go — the hasura image per compat mode (EPIC D17, P7-LIVE-23).
//
// Purpose: prove that v1.4 runs graphql-engine v2.44.0 (hasura-v14), that v1.5
// runs v2.51.0 (hasura), that an explicit HASURA_VERSION wins, and that a
// project lock pin of v2.44.0 survives v1.5 (the pin is what keeps an existing
// project on the version its database was migrated by).
// Inputs: minimalConfig plus the embedded image lock; digests are the lock's.
// Outputs: test failures naming mode, case and image.
// Constraints: no network, no Docker.

import (
	"bytes"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func hasuraImageFor(t *testing.T, version string) string {
	t.Helper()
	cfg := minimalConfig()
	cfg.Hasura.Version = version
	svc, err := NewGenerator(cfg).buildHasuraService()
	if err != nil {
		t.Fatalf("buildHasuraService: %v", err)
	}
	return svc.Image
}

func TestHasuraImageByMode(t *testing.T) {
	v14, ok1 := LockedRef("hasura-v14")
	cur, ok2 := LockedRef("hasura")
	if !ok1 || !ok2 {
		t.Fatal("the image lock lacks hasura or hasura-v14")
	}
	if v14.Version != "v2.44.1" || cur.Version != "v2.51.2" || v14.Repository != cur.Repository || v14.IndexDigest == cur.IndexDigest {
		t.Fatalf("lock entries hasura-v14 = %s, hasura = %s: want v2.44.1 and v2.51.2 of one repository with distinct digests", v14.String(), cur.String())
	}
	restore := SetProjectPins(nil)
	defer restore()
	cases := []struct {
		name, version string
		v15           bool
		mode          string // pinning mode
		want          string
	}{
		{"v1.4 lock default", "v2.44.1", false, PinningLock, v14.String()},
		{"v1.4 lock blank version", "", false, PinningLock, v14.String()},
		{"v1.4 legacy default", "v2.44.0", false, PinningLegacy, "hasura/graphql-engine:v2.44.0"},
		{"v1.5 explicit v1.4 release", "v2.44.1", true, PinningLock, v14.String()},
		{"v1.5 lock blank version", "", true, PinningLock, cur.String()},
		{"v1.5 explicit v1.4 legacy release", "v2.44.0", true, PinningLegacy, "hasura/graphql-engine:v2.44.0"},
		{"explicit current release in v1.4", "v2.51.2", false, PinningLock, cur.String()},
		{"explicit current release in v1.4 legacy", "v2.51.0", false, PinningLegacy, "hasura/graphql-engine:v2.51.0"},
		{"explicit older release in v1.4", "v2.43.0", false, PinningLock, "hasura/graphql-engine:v2.43.0"},
		{"explicit older release in v1.5", "v2.43.0", true, PinningLock, "hasura/graphql-engine:v2.43.0"},
		{"explicit newer release in v1.5", "v2.52.0", true, PinningLock, "hasura/graphql-engine:v2.52.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			setMode(t, tc.mode)
			if got := hasuraImageFor(t, tc.version); got != tc.want {
				t.Errorf("hasura image = %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("HasuraEntry follows the mode", func(t *testing.T) {
		compattest.Both(t, func(t *testing.T) {
			want := "hasura-v14"
			if compat.V15() {
				want = "hasura"
			}
			if got := HasuraEntry(); got != want {
				t.Errorf("HasuraEntry() = %q, want %q", got, want)
			}
		})
	})
}

func TestHasuraLockPinKept(t *testing.T) {
	v14, _ := LockedRef("hasura-v14")
	cur, _ := LockedRef("hasura")
	pin := v14.String() // an existing project: its lock pins v2.44.0
	compattest.Both(t, func(t *testing.T) {
		setMode(t, PinningLock)
		var notices bytes.Buffer
		defer PinNoticeSink(&notices)()
		defer SetProjectPins(map[string]string{"hasura": pin})()
		// Blank keeps the pin; an explicit version matching the pin keeps its digest.
		for _, v := range []string{"", v14.Version} {
			t.Setenv("HASURA_VERSION", v)
			if got := hasuraImageFor(t, v); got != pin {
				t.Errorf("HASURA_VERSION=%q: hasura image = %q, want the pin %q", v, got, pin)
			}
		}
		if notices.Len() != 0 {
			t.Errorf("a kept pin printed a notice: %q", notices.String())
		}
		// A different explicit version wins over the pin with one notice.
		for v, want := range map[string]string{cur.Version: cur.String(), "v2.45.0": "hasura/graphql-engine:v2.45.0"} {
			notices.Reset()
			resetPinNotices(t)
			t.Setenv("HASURA_VERSION", v)
			if got := hasuraImageFor(t, v); got != want {
				t.Errorf("explicit HASURA_VERSION=%s: hasura image = %q, want %q", v, got, want)
			}
			if notices.Len() == 0 {
				t.Errorf("HASURA_VERSION=%s dropped the pin without a notice", v)
			}
		}
	})
	t.Run("no pin renders the mode default", func(t *testing.T) {
		setMode(t, PinningLock)
		defer SetProjectPins(nil)()
		compattest.Set(t, false)
		if got := hasuraImageFor(t, ""); got != v14.String() {
			t.Errorf("v1.4 without a pin: %q, want %q", got, v14.String())
		}
		compattest.Set(t, true)
		if got := hasuraImageFor(t, ""); got != cur.String() {
			t.Errorf("v1.5 without a pin: %q, want %q", got, cur.String())
		}
	})
}

// TestHasuraLockPinKeptAfterUpdate: a project `nself update` moved to v2.51.0 keeps
// that pin when .env leaves the version blank or names the same release.
// Every nonblank HASURA_VERSION is explicit and overrides the pin, including
// v2.44.0 written by `nself service upgrade hasura v2.44.0`.
func TestHasuraLockPinKeptAfterUpdate(t *testing.T) {
	cur, _ := LockedRef("hasura")
	v14, _ := LockedRef("hasura-v14")
	compattest.Both(t, func(t *testing.T) {
		setMode(t, PinningLock)
		var notices bytes.Buffer
		defer PinNoticeSink(&notices)()
		defer SetProjectPins(map[string]string{"hasura": cur.String()})()
		// Blank and the pin's own version keep the v2.51.0 pin silently.
		for _, v := range []string{"", cur.Version} {
			t.Setenv("HASURA_VERSION", v)
			if got := hasuraImageFor(t, v); got != cur.String() {
				t.Errorf("HASURA_VERSION=%q: hasura image = %q, want the pin %q", v, got, cur.String())
			}
		}
		if notices.Len() != 0 {
			t.Errorf("a kept pin printed a notice: %q", notices.String())
		}
		// v2.44.0 is explicit: it overrides the v2.51.0 pin with a notice.
		notices.Reset()
		t.Setenv("HASURA_VERSION", v14.Version)
		want := v14.String()
		if got := hasuraImageFor(t, v14.Version); got != want {
			t.Errorf("HASURA_VERSION=%q: hasura image = %q, want %q", v14.Version, got, want)
		}
		if notices.Len() == 0 {
			t.Error("expected a pin-drop notice for explicit v2.44.0 override")
		}
	})
}

// TestHasuraInitUpgradeWithoutPin: without a project pin, HASURA_VERSION=v2.44.0
// remains explicit even in v1.5 — the migration only applies when a v2.51.0 pin
// exists (nself update case). This test documents that boundary.
func TestHasuraInitUpgradeKeepsNewerPin(t *testing.T) {
	v14, _ := LockedRef("hasura-v14")
	setMode(t, PinningLock)
	defer SetProjectPins(nil)()
	compattest.Set(t, true)
	t.Setenv("HASURA_VERSION", v14.Version)
	// Without a pin, v2.44.0 is explicit and resolves to hasura-v14.
	if got := hasuraImageFor(t, v14.Version); got != v14.String() {
		t.Errorf("v1.5 without pin, HASURA_VERSION=%s: hasura image = %q, want explicit v14 %q", v14.Version, got, v14.String())
	}
	// v1.4 also treats v2.44.0 as explicit (it matches hasura-v14's default).
	compattest.Set(t, false)
	if got := hasuraImageFor(t, v14.Version); got != v14.String() {
		t.Errorf("v1.4 without pin, HASURA_VERSION=%s: hasura image = %q, want explicit v14 %q", v14.Version, got, v14.String())
	}
}
