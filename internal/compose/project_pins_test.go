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
	"github.com/nself-org/cli/internal/config"
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
	envs := map[string]string{"POSTGRES_IMAGE": "postgres", "NSELF_ADMIN_VERSION": "admin", "MEILISEARCH_VERSION": "meilisearch", "POSTGRES_VERSION": "postgres"}
	pins := map[string]string{}
	for _, svc := range envs {
		pins[svc] = CanonicalRef(DefaultImage(svc)) + "-pin" // same repository, another tag
	}
	defer SetProjectPins(pins)()
	for env, svc := range envs {
		image := DefaultImage(svc)
		if _, ok := projectPin(svc, image); !ok {
			t.Fatalf("%s: pin not applied with %s unset", svc, env)
		}
		t.Setenv(env, "x")
		if _, ok := projectPin(svc, image); ok {
			t.Errorf("%s set but the pin still applies to %s", env, svc)
		}
		_ = os.Unsetenv(env)
	}
	// POSTGRES_EXTENSIONS is not a version override: the rendered image decides.
	t.Setenv("POSTGRES_EXTENSIONS", "pgvector")
	if _, ok := projectPin("postgres", DefaultImage("postgres")); !ok {
		t.Error("POSTGRES_EXTENSIONS dropped the postgres pin")
	}
}

// TestCLIOwnedPrefersCatalogServiceOnPinKeyCollision proves that a user
// service named like a catalog pin key cannot replace the catalog service in
// the project lock. The admin UI is rendered as nself-admin but pinned as
// "admin", so both names otherwise collapse onto the same map key.
func TestCLIOwnedPrefersCatalogServiceOnPinKeyCollision(t *testing.T) {
	const catalogRef = "nself/nself-admin:catalog"
	all := map[string]string{
		"admin":          "example.com/user/admin:custom",
		AdminServiceName: catalogRef,
	}
	for i := 0; i < 256; i++ {
		if got := CLIOwned(all)["admin"]; got != catalogRef {
			t.Fatalf("iteration %d: admin pin = %q, want catalog service %q", i, got, catalogRef)
		}
	}
}

// TestProjectPinOtherEntryDefaultIsAnOverride covers a service with multiple
// image-lock entries. pg16 is pgvector's default tag, but when plain postgres
// is rendered it is an explicit postgres version and must drop the old pin.
func TestProjectPinOtherEntryDefaultIsAnOverride(t *testing.T) {
	t.Setenv(EnvImagePinning, PinningLegacy)
	t.Setenv("POSTGRES_VERSION", "pg16")
	pin := "postgres:15-alpine@" + pinDigest
	defer SetProjectPins(map[string]string{"postgres": pin})()
	var notices bytes.Buffer
	defer PinNoticeSink(&notices)()

	rendered := ResolvePostgresImage(config.PostgresConfig{Version: "pg16"})
	got := ResolveImage("postgres", rendered)
	if got == pin || CanonicalRef(got) != "postgres:pg16" {
		t.Fatalf("POSTGRES_VERSION=pg16 must drop the plain-postgres pin: got %q", got)
	}
	if n := strings.Count(notices.String(), "notice:"); n != 1 || !strings.Contains(notices.String(), "POSTGRES_VERSION is set") {
		t.Fatalf("want one notice naming POSTGRES_VERSION, got %q", notices.String())
	}
}

// TestImageInputDefaultCompatibilityIsDirectional keeps the explicit
// generator mappings narrow. A selected row may accept a compatibility default
// that the generator translates to it, but the reverse direction is not
// implied just because both rows belong to one compose service.
func TestImageInputDefaultCompatibilityIsDirectional(t *testing.T) {
	rows := map[string]ImageInput{}
	for _, in := range ImageInputs() {
		rows[in.Entry] = in
	}
	postgres, _ := LockedRef("postgres")
	pgvector, _ := LockedRef("pgvector")
	redis, _ := LockedRef("redis")
	valkey, _ := LockedRef("valkey")
	v14, _ := LockedRef("hasura-v14")
	current, _ := LockedRef("hasura")
	for _, tc := range []struct {
		name, entry, value string
		want               bool
	}{
		{"pgvector accepts postgres init default", "pgvector", postgres.Version, true},
		{"postgres rejects pgvector default", "postgres", pgvector.Version, false},
		{"valkey accepts redis init default", "valkey", redis.Version, true},
		{"redis rejects valkey default", "redis", valkey.Version, false},
		{"current hasura accepts init default", "hasura", v14.Version, true},
		{"current hasura remains explicit", "hasura", current.Version, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := rows[tc.entry].isDefault(tc.value); got != tc.want {
				t.Fatalf("%s.isDefault(%q) = %v, want %v", tc.entry, tc.value, got, tc.want)
			}
		})
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

// resetPinNotices clears the process-wide notice dedupe for a test.
func resetPinNotices(t *testing.T) {
	t.Helper()
	pinNoticedMu.Lock()
	pinNoticed = map[string]bool{}
	pinNoticedMu.Unlock()
}

// TestProjectPinEnvEqualToPin: nself init writes *_VERSION defaults into .env.
// A value that renders to the pin's reference keeps ref@digest, in both pinning
// modes, whether the pin was bootstrapped in the short or the docker.io/
// spelling (lens round 3 must-fix; CR r3 S1 for postgres), and a different one
// drops the pin with a single notice.
func TestProjectPinEnvEqualToPin(t *testing.T) {
	type svc struct {
		name, env, version string
		render             func(version string) string
	}
	pgCfg := func(v string, ext ...string) string {
		return ResolvePostgresImage(config.PostgresConfig{Version: v, Extensions: ext})
	}
	for _, mode := range []string{PinningLegacy, PinningLock} {
		for _, spelling := range []string{"short", "docker.io"} {
			for _, sv := range []svc{
				{"hasura", "HASURA_VERSION", "v2.44.0", func(v string) string { return ImageRef("hasura", v) }},
				{"auth", "AUTH_VERSION", "", func(v string) string { return ImageRef("auth", v) }},
				{"nginx", "NGINX_VERSION", "", func(string) string { return ImageRef("nginx", "") }},
				{"redis", "REDIS_VERSION", "", func(v string) string { return ImageRef("redis", v) }},
				{"postgres", "POSTGRES_VERSION", "16-alpine", func(v string) string { return pgCfg(v, "uuid-ossp", "pgvector") }},
			} {
				name := mode + "/" + spelling + "/" + sv.name + "/" + sv.env
				t.Run(name, func(t *testing.T) {
					t.Setenv(EnvImagePinning, mode)
					for _, v15 := range []bool{false, true} {
						compattest.Set(t, v15)
						version := sv.version
						if r, ok := LockedRef(sv.name); ok && version == "" {
							version = r.Version
						}
						// The pin: the reference the project already runs.
						base := sv.render(version)
						if sv.name == "postgres" {
							base = ImageRef("pgvector", "pg16")
						}
						base = CanonicalRef(base)
						if spelling == "docker.io" {
							base = "docker.io/" + base
						}
						pin := base + "@" + pinDigest
						restore := SetProjectPins(map[string]string{sv.name: pin})
						resetPinNotices(t)
						var notice bytes.Buffer
						prevW := pinNoticeW
						pinNoticeW = &notice
						t.Cleanup(func() { pinNoticeW = prevW; restore() })

						t.Setenv(sv.env, version)
						got := ResolveImage(sv.name, sv.render(version))
						if got != pin {
							t.Errorf("v15=%v: %s=%s dropped the pin: got %q want %q", v15, sv.env, version, got, pin)
						}
						if notice.Len() != 0 {
							t.Errorf("v15=%v: a kept pin must not print a notice: %q", v15, notice.String())
						}
					}
				})
			}
		}
	}
}

// TestProjectPinEnvDifferentDropsPin: a value that renders to another image
// drops the pin, with exactly one notice per build however many times the pins
// are installed and resolved (CR r3 S2).
func TestProjectPinEnvDifferentDropsPin(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		compattest.Set(t, v15)
		pin := "hasura/graphql-engine:v2.44.0@" + pinDigest
		resetPinNotices(t)
		var notice bytes.Buffer
		prevW := pinNoticeW
		pinNoticeW = &notice
		t.Cleanup(func() { pinNoticeW = prevW })

		t.Setenv("HASURA_VERSION", "v2.51.0")
		for i := 0; i < 3; i++ {
			restore := SetProjectPins(map[string]string{"hasura": pin}) // a build installs the pins repeatedly
			if got := ResolveImage("hasura", ImageRef("hasura", "v2.51.0")); got == pin || !strings.Contains(got, "v2.51.0") {
				t.Errorf("v15=%v: a different explicit version kept the pin: %q", v15, got)
			}
			restore()
		}
		if n := strings.Count(notice.String(), "notice:"); n != 1 || !strings.Contains(notice.String(), "HASURA_VERSION") {
			t.Errorf("v15=%v: want one notice naming HASURA_VERSION, got %q", v15, notice.String())
		}
	}
}

// TestProjectPinRepositoryMustMatch: a pin applies only to the repository it
// was taken from, so a runtime switch or pgvector turned off renders the new
// image (adversarial M3), and deno, python and meilisearch-init honour pins.
func TestProjectPinRepositoryMustMatch(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	for _, mode := range []string{PinningLegacy, PinningLock} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(EnvImagePinning, mode)
			cfgs := fixtureConfigs(t)
			deno := "denoland/deno:1.40.0@" + pinDigest
			busy := "busybox:1.30@" + pinDigest
			pgv := "pgvector/pgvector:pg16@" + pinDigest
			resetPinNotices(t)
			prev := pinNoticeW
			pinNoticeW = &bytes.Buffer{}
			defer func() { pinNoticeW = prev }()

			has := func(cfg string, want string) bool {
				for _, g := range generatedImages(t, cfgs[cfg]) {
					if g == want {
						return true
					}
				}
				return false
			}
			defer SetProjectPins(map[string]string{"functions": deno, "meilisearch-init": busy, "postgres": pgv})()
			if !has("search-typesense-deno", deno) {
				t.Errorf("the deno functions runtime ignored its pin: %v", generatedImages(t, cfgs["search-typesense-deno"]))
			}
			if !has("all-optional", busy) {
				t.Errorf("meilisearch-init ignored its pin: %v", generatedImages(t, cfgs["all-optional"]))
			}
			// all-optional runs the node functions runtime: the deno pin must not apply.
			if has("all-optional", deno) {
				t.Errorf("a deno pin hid the switch to the node runtime")
			}
			if !has("python-pgvector", pgv) {
				t.Errorf("the pgvector pin was not kept for a pgvector project: %v", generatedImages(t, cfgs["python-pgvector"]))
			}
			// dev-minimal renders plain postgres (no pgvector): the pgvector pin must not apply.
			if has("dev-minimal", pgv) {
				t.Errorf("a pgvector pin kept the pgvector image after pgvector was switched off")
			}
		})
	}
}

// TestCanonicalRefTable: one spelling for comparison; host and path lower case,
// the tag exactly as written because registries treat tags case-sensitively
// (adversarial r2 N3).
func TestCanonicalRefTable(t *testing.T) {
	for in, want := range map[string]string{
		"x":                                "x",
		"library/x":                        "x",
		"docker.io/library/x:1":            "x:1",
		"index.docker.io/library/x:1":      "x:1",
		"registry-1.docker.io/library/x:1": "x:1",
		"x@" + pinDigest:                   "x",
		"hasura/graphql-engine:v2.44.0@" + pinDigest: "hasura/graphql-engine:v2.44.0",
		"Hasura/Graphql-Engine:v2.44.0":              "hasura/graphql-engine:v2.44.0",
		"hasura/graphql-engine:V2.44.0":              "hasura/graphql-engine:V2.44.0",
		"DOCKER.IO/Library/App:Latest-RC1":           "app:Latest-RC1",
		"localhost:5000/X":                           "localhost:5000/x",
		"LocalHost:5000/X:Tag":                       "localhost:5000/x:Tag",
		"ghcr.io/library/x:1":                        "ghcr.io/library/x:1",
		"  x:1  ":                                    "x:1",
	} {
		if got := CanonicalRef(in); got != want {
			t.Errorf("CanonicalRef(%q) = %q, want %q", in, got, want)
		}
	}
	if CanonicalRef("app:V1") == CanonicalRef("app:v1") {
		t.Error("tags differing only in case are different tags")
	}
}
