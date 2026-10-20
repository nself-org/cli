package commands

// start_lock_pins_init_test.go — project lock pins against the .env that the real
// nself init writes (P7-LIVE-09, round 9).
//
// Purpose: a CLI upgrade must not move an image that the project's lock pins
// just because .env holds the default version init wrote into it. The test runs
// setup.Initialize (the code behind nself init), renders the stack from the .env
// it produced, pins every CLI-owned service at an older tag, and expects the
// literal outcome: every pin survives with no notice. Editing one default to
// another version drops that one pin with one notice.
// Inputs: setup.Initialize in a temp dir, in legacy and lock pinning mode.
// Outputs: failures naming the service and the rendered image.
// Constraints: hermetic; nothing here re-implements the pin rule.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/setup"
)

// olderPin returns a pin for the image img: its repository at an older tag.
func olderPin(img string) string {
	base, _, _ := strings.Cut(img, "@")
	if i := strings.LastIndex(base, ":"); i > strings.LastIndex(base, "/") {
		base = base[:i]
	}
	return base + ":0.0.1-older@" + matrixPinDigest
}

func TestProjectLockPinsSurviveInitDefaults(t *testing.T) {
	for _, mode := range []string{"legacy", "lock"} {
		t.Run(mode, func(t *testing.T) {
			blankImageVars(t)
			t.Setenv("IMAGE_PINNING", mode)
			t.Setenv("NSELF_ADMIN_ENABLED", "true")
			dir := t.TempDir()
			if _, err := setup.Initialize(setup.Options{WorkDir: dir, Name: "pinproj", Demo: true, NonInteractive: true, Quiet: true}); err != nil {
				t.Fatalf("nself init: %v", err)
			}

			restore := compose.SetProjectPins(nil)
			plain := renderStackIn(t, dir, nil)
			restore()
			pins := map[string]string{}
			for name, img := range plain {
				if compose.IsCLIOwned(name) {
					pins[compose.PinKey(name)] = olderPin(img)
				}
			}
			for _, key := range []string{"postgres", "hasura", "auth", "redis", "admin"} {
				if pins[key] == "" {
					t.Fatalf("the init stack has no %s to pin: %v", key, plain)
				}
			}

			defer compose.SetProjectPins(pins)()
			var notices bytes.Buffer
			defer compose.PinNoticeSink(&notices)()
			got := renderStackIn(t, dir, nil)
			for name := range plain {
				if pin, ok := pins[compose.PinKey(name)]; ok && got[name] != pin {
					t.Errorf("%s: the pin %s was moved by the init defaults: image %s", name, pin, got[name])
				}
			}
			if notices.Len() != 0 {
				t.Errorf("init defaults must not print a drop notice: %q", notices.String())
			}

			// One default changed to another version is an override: that pin drops.
			envPath := filepath.Join(dir, ".env")
			raw, err := os.ReadFile(envPath)
			if err != nil {
				t.Fatal(err)
			}
			edited := strings.Replace(string(raw), "HASURA_VERSION=\n", "HASURA_VERSION=v2.51.0\n", 1)
			if edited == string(raw) {
				t.Fatal("init no longer writes HASURA_VERSION=: update this test with the new default")
			}
			if err := os.WriteFile(envPath, []byte(edited), 0o600); err != nil {
				t.Fatal(err)
			}
			got = renderStackIn(t, dir, nil)
			if got["hasura"] == pins["hasura"] || !strings.Contains(got["hasura"], "v2.51.0") {
				t.Errorf("HASURA_VERSION=v2.51.0 must drop the hasura pin: image %s", got["hasura"])
			}
			if n := strings.Count(notices.String(), "notice:"); n != 1 || !strings.Contains(notices.String(), "HASURA_VERSION is set") {
				t.Errorf("want one notice naming HASURA_VERSION, got %q", notices.String())
			}
			if got["redis"] != pins["redis"] {
				t.Errorf("an unrelated pin moved: redis %s", got["redis"])
			}
		})
	}
}
