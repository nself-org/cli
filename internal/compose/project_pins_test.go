package compose

// project_pins_test.go — project lock pins in ResolveImage (P7-LIVE-09):
// precedence explicit env > lock pin > CLI image lock, restore, file loading.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

const pinDigest = "sha256:4444444444444444444444444444444444444444444444444444444444444444"

func TestProjectPinsPrecedence(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		compattest.Set(t, v15)
		def := ResolveImage("hasura", ImageRef("hasura", "v2.44.0"))
		pin := "hasura/graphql-engine:v2.40.0@" + pinDigest
		restore := SetProjectPins(map[string]string{"hasura": pin})

		// Lock pin beats the CLI default and a version the config defaulted.
		if got := ResolveImage("hasura", ImageRef("hasura", "v2.44.0")); got != pin {
			t.Errorf("v15=%v: pin lost to the default: %q", v15, got)
		}
		if got := ResolveImage("hasura", ""); got != pin {
			t.Errorf("v15=%v: pin lost to DefaultImage: %q", v15, got)
		}
		// A service with no pin is untouched.
		if got, want := ResolveImage("redis", ImageRef("redis", "")), ImageRef("redis", ""); got != want {
			t.Errorf("v15=%v: unpinned service changed: %q != %q", v15, got, want)
		}
		// An explicit version in the env beats the pin.
		t.Setenv("HASURA_VERSION", "v2.41.0")
		if got := ResolveImage("hasura", ImageRef("hasura", "v2.41.0")); got == pin || got != "hasura/graphql-engine:v2.41.0" {
			t.Errorf("v15=%v: explicit HASURA_VERSION lost to the pin: %q", v15, got)
		}
		_ = os.Unsetenv("HASURA_VERSION")

		restore()
		if got := ResolveImage("hasura", ImageRef("hasura", "v2.44.0")); got != def {
			t.Errorf("v15=%v: restore did not clear the pin: %q != %q", v15, got, def)
		}
	}
}

func TestProjectPinsExplicitNames(t *testing.T) {
	restore := SetProjectPins(map[string]string{"postgres": "postgres:15", "admin": "nself/nself-admin:1", "meilisearch": "m:1"})
	defer restore()
	for env, svc := range map[string]string{"POSTGRES_IMAGE": "postgres", "POSTGRES_EXTENSIONS": "postgres", "NSELF_ADMIN_VERSION": "admin", "MEILISEARCH_VERSION": "meilisearch"} {
		if _, ok := projectPin(svc); !ok {
			t.Fatalf("%s: pin not applied with %s unset", svc, env)
		}
		t.Setenv(env, "x")
		if _, ok := projectPin(svc); ok {
			t.Errorf("%s set but the pin still applies to %s", env, svc)
		}
		_ = os.Unsetenv(env)
	}
}

func TestProjectPinsLoad(t *testing.T) {
	dir := t.TempDir()
	if err := LoadProjectPins(dir); err != nil || len(ProjectPins()) != 0 {
		t.Fatalf("a project without a lock has no pins: %v %v", err, ProjectPins())
	}
	body := "# GENERATED\nschema_version: 1\ncli: \"1.5\"\nimages:\n  redis: redis:7-alpine@" + pinDigest + "\nunpinned:\n  hasura: hasura/graphql-engine:v2.40.0\nplugins: {}\nbundles: []\n"
	if err := os.WriteFile(filepath.Join(dir, ProjectLockFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { SetProjectPins(nil) })
	if err := LoadProjectPins(dir); err != nil {
		t.Fatal(err)
	}
	p := ProjectPins()
	if p["redis"] != "redis:7-alpine@"+pinDigest || p["hasura"] != "hasura/graphql-engine:v2.40.0" || len(p) != 2 {
		t.Fatalf("pins = %v", p)
	}
	p["redis"] = "mutated"
	if ProjectPins()["redis"] == "mutated" {
		t.Fatal("ProjectPins returned the live map")
	}
	if err := os.WriteFile(filepath.Join(dir, ProjectLockFile), []byte("images: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := LoadProjectPins(dir); err == nil {
		t.Fatal("a damaged lock must be an error")
	}
}

// TestProjectPinEnvEqualToPin: nself init writes *_VERSION defaults into .env;
// a value equal to the pin's reference keeps ref@digest, a different one drops
// the pin with a single notice (lens must-fix 2).
func TestProjectPinEnvEqualToPin(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		v15 := v15
		t.Run(map[bool]string{false: "v14", true: "v15"}[v15], func(t *testing.T) { testProjectPinEnvEqualToPin(t, v15) })
	}
}

func testProjectPinEnvEqualToPin(t *testing.T, v15 bool) {
	{
		compattest.Set(t, v15)
		pin := "hasura/graphql-engine:v2.44.0@" + pinDigest
		restore := SetProjectPins(map[string]string{"hasura": pin})
		var notice bytes.Buffer
		prevW := pinNoticeW
		pinNoticeW = &notice
		t.Cleanup(func() { pinNoticeW = prevW })

		t.Setenv("HASURA_VERSION", "v2.44.0")
		if got := ResolveImage("hasura", ImageRef("hasura", "v2.44.0")); got != pin {
			t.Errorf("v15=%v: HASURA_VERSION equal to the pin dropped the digest: %q", v15, got)
		}
		if notice.Len() != 0 {
			t.Errorf("v15=%v: equal value must not print a notice: %q", v15, notice.String())
		}

		t.Setenv("HASURA_VERSION", "v2.51.0")
		for i := 0; i < 2; i++ {
			if got := ResolveImage("hasura", ImageRef("hasura", "v2.51.0")); got == pin {
				t.Errorf("v15=%v: a different explicit version kept the pin", v15)
			}
		}
		if n := strings.Count(notice.String(), "notice:"); n != 1 || !strings.Contains(notice.String(), "HASURA_VERSION") {
			t.Errorf("v15=%v: want one notice naming HASURA_VERSION, got %q", v15, notice.String())
		}

		t.Setenv("HASURA_VERSION", "")
		t.Setenv("HASURA_IMAGE", "hasura/graphql-engine:v2.44.0")
		if got, ok := projectPin("hasura"); !ok || got != pin {
			t.Errorf("v15=%v: HASURA_IMAGE equal to the pin base dropped it: %q %v", v15, got, ok)
		}
		t.Setenv("HASURA_IMAGE", "other/engine:1")
		if _, ok := projectPin("hasura"); ok {
			t.Errorf("v15=%v: a different HASURA_IMAGE kept the pin", v15)
		}
		restore()
	}
}
