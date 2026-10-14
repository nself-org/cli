package build

// socket_guard_test.go — P7-DEPL-21 (ADR 0027 §3): the build refuses a plugin
// fragment or generated stack that exposes the Docker socket (E130), an
// excepted plugin warns once until its removal version, and the exception
// table expires. Fragments are synthetic files under testdata/socket/; the
// plugin cases run through Build, the real orchestrator path.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/version"
)

// setCLIVersion sets the running CLI version for one test.
func setCLIVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

// installSocketFragment copies a testdata/socket fixture into the fixture's
// plugin directory as <slug>/docker-compose.plugin.yml.
func installSocketFragment(t *testing.T, f *planFixture, slug, fixture string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "socket", fixture))
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(f.plugins, slug, pluginComposeFilename), string(data), 0o644)
}

// warnedCount is the number of distinct fragment warnings printed so far.
func warnedCount() int {
	n := 0
	fragmentWarned.Range(func(_, _ any) bool { n++; return true })
	return n
}

func TestSocketGuardFragments(t *testing.T) {
	setCLIVersion(t, "1.4.14")
	cases := []struct {
		name, fixture, slug string
		wantService         string // "" = the build passes
	}{
		{"no socket", "clean.yml", "sockclean", ""},
		{"remote tcp DOCKER_HOST is not a mount", "tcp_host.yml", "socktcp", ""},
		{"short bind", "mount_short.yml", "sockshort", "sockshort"},
		{"long bind", "mount_long.yml", "socklong", "socklong"},
		{"interpolated path", "mount_interpolated.yml", "sockvar", "sockvar"},
		{"parent directory", "mount_parent_dir.yml", "sockdir", "sockdir"},
		{"named volume backed by the socket", "mount_named_volume.yml", "socknamed", "socknamed"},
		{"unix DOCKER_HOST list form", "docker_host_list.yml", "sockenvlist", "sockenvlist"},
		{"unix DOCKER_HOST map form", "docker_host_map.yml", "sockenvmap", "sockenvmap"},
		{"second service of the fragment", "second_service.yml", "sockpair", "socksecond"},
		{"unparseable fragment is refused", "invalid.yml", "sockbroken", "cannot be checked"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPlanFixture(t, "dev-minimal")
			installSocketFragment(t, f, tc.slug, tc.fixture)
			_, err := Build(f.workdir, BuildOptions{Force: true})
			if tc.wantService == "" {
				if err != nil {
					t.Fatalf("build refused a clean fragment: %v", err)
				}
				return
			}
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E130" {
				t.Fatalf("want E130, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.slug) || !strings.Contains(err.Error(), tc.wantService) {
				t.Errorf("E130 must name plugin %q and %q: %v", tc.slug, tc.wantService, err)
			}
		})
	}

	t.Run("excepted plugin warns once across builds", func(t *testing.T) {
		resetWarnOnce()
		f := newPlanFixture(t, "dev-minimal")
		installSocketFragment(t, f, "forgejo", "mount_short.yml")
		for i := 0; i < 2; i++ {
			if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
				t.Fatalf("excepted plugin refused on build %d: %v", i+1, err)
			}
		}
		if n := warnedCount(); n != 1 {
			t.Errorf("want exactly one warning, got %d", n)
		}
	})

	t.Run("an exception covers its own plugin only", func(t *testing.T) {
		f := newPlanFixture(t, "dev-minimal")
		installSocketFragment(t, f, "forgejo", "mount_short.yml")
		installSocketFragment(t, f, "other", "docker_host_map.yml")
		_, err := Build(f.workdir, BuildOptions{Force: true})
		if err == nil || !strings.Contains(err.Error(), `"other"`) {
			t.Fatalf("want E130 naming other, got %v", err)
		}
	})

	t.Run("a disabled plugin is not checked", func(t *testing.T) {
		f := newPlanFixture(t, "dev-minimal")
		installSocketFragment(t, f, "sockoff", "mount_short.yml")
		writeFixtureFile(t, filepath.Join(f.plugins, "sockoff", ".disabled"), "", 0o644)
		if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
			t.Fatalf("disabled plugin refused: %v", err)
		}
	})

	t.Run("generated stack", func(t *testing.T) {
		const stack = `services:
  nginx:
    image: example.invalid/nginx:1
    volumes: [{type: bind, source: /var/run/docker.sock, target: /var/run/docker.sock}]
  nself-admin:
    image: example.invalid/admin:1
    environment: {DOCKER_HOST: "unix:///var/run/docker.sock"}
    volumes: ["/var/run/docker.sock:/var/run/docker.sock:ro"]
  ping-api:
    image: example.invalid/custom:1
    volumes: ["/var/run/docker.sock:/var/run/docker.sock"]
`
		skip := map[string]bool{"nself-admin": true, "ping-api": true}
		err := guardGeneratedStack([]byte(stack), skip)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E130" || !strings.Contains(err.Error(), `"nginx"`) {
			t.Fatalf("want E130 naming nginx, got %v", err)
		}
		only := strings.Replace(stack, "  nginx:", "  nginxx:", 1)
		if err := guardGeneratedStack([]byte(only), map[string]bool{"nself-admin": true, "ping-api": true, "nginxx": true}); err != nil {
			t.Errorf("admin, custom and skipped services must pass: %v", err)
		}
		if err := guardGeneratedStack([]byte("services: [unclosed"), nil); err == nil {
			t.Error("an unreadable stack must not pass")
		}
	})
}

func TestSocketGuardExceptionExpiry(t *testing.T) {
	t.Run("table entries all expire", func(t *testing.T) {
		for slug, ex := range socketExceptions {
			if _, ok := parseXYZ(ex.RemovalAt); !ok || ex.Slug != slug || ex.Ticket == "" {
				t.Errorf("%s: removal_at %q ticket %q slug %q is not a usable, expiring entry", slug, ex.RemovalAt, ex.Ticket, ex.Slug)
			}
		}
		if socketExceptions["forgejo"].Ticket != "P7-TRUST-21" || socketExceptions["monitoring"].Ticket != "P7-PLUG-58" {
			t.Error("forgejo and monitoring must map to P7-TRUST-21 and P7-PLUG-58")
		}
	})

	removal := socketExceptions["forgejo"].RemovalAt
	cases := []struct {
		version string
		active  bool
	}{
		{"1.4.14", true},
		{"1.4.99", true},
		{"v1.4.0", true},
		{removal, false},
		{removal + "-rc.1", false},
		{"1.5.1", false},
		{"2.0.0", false},
		{"dev", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run("version "+tc.version, func(t *testing.T) {
			setCLIVersion(t, tc.version)
			for _, slug := range []string{"forgejo", "monitoring"} {
				if _, ok := ActiveSocketException(slug); ok != tc.active {
					t.Errorf("%s at %q: active=%v want %v", slug, tc.version, ok, tc.active)
				}
			}
		})
	}

	t.Run("unknown slug is never excepted", func(t *testing.T) {
		setCLIVersion(t, "1.0.0")
		if _, ok := ActiveSocketException("nobody"); ok {
			t.Error("a slug outside the table was excepted")
		}
	})

	t.Run("build stops allowing the mount at the removal version", func(t *testing.T) {
		for _, slug := range []string{"forgejo", "monitoring"} {
			f := newPlanFixture(t, "dev-minimal")
			installSocketFragment(t, f, slug, "mount_short.yml")
			setCLIVersion(t, "1.4.14")
			if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
				t.Fatalf("%s refused before removal: %v", slug, err)
			}
			setCLIVersion(t, removal)
			_, err := Build(f.workdir, BuildOptions{Force: true})
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E130" || !strings.Contains(err.Error(), slug) {
				t.Fatalf("%s at %s: want E130, got %v", slug, removal, err)
			}
		}
	})
}
