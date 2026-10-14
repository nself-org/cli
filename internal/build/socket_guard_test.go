package build

// socket_guard_test.go — P7-DEPL-21 (ADR 0027 §3): the build refuses a plugin
// fragment, override file or generated stack that gives a proxy or plugin the
// Docker socket (E130), an excepted plugin warns once until its removal
// version, and the exception table expires. Fragments are synthetic files
// under testdata/socket/; the plugin cases run through Build, the real
// orchestrator path. The fixture sweep fails when a fixture is never used.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
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

// warnedCount is the number of distinct fragment warnings printed so far.
func warnedCount() int {
	n := 0
	fragmentWarned.Range(func(_, _ any) bool { n++; return true })
	return n
}

func ptr(s string) *string { return &s }

// activeAt reports whether slug's exception still applies at CLI version v.
func activeAt(slug, v string) bool {
	old := version.Version
	version.Version = v
	defer func() { version.Version = old }()
	_, ok := ActiveSocketException(slug)
	return ok
}

// socketFixture reads a testdata/socket fixture and records that it was used.
func socketFixture(t *testing.T, used map[string]bool, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "socket", name))
	if err != nil {
		t.Fatal(err)
	}
	used[name] = true
	return string(data)
}

type socketCase struct {
	name     string
	plugins  map[string]string // plugin slug -> fixture
	override string            // fixture installed as docker-compose.override.yml
	env      map[string]string
	want     []string // substrings of the E130 message; nil = the build passes
}

// buildSocketCase lays the case out in a fresh project and runs Build.
func buildSocketCase(t *testing.T, used map[string]bool, tc socketCase) error {
	t.Helper()
	f := newPlanFixture(t, "dev-minimal")
	for k, v := range tc.env {
		t.Setenv(k, v)
	}
	for slug, fixture := range tc.plugins {
		writeFixtureFile(t, filepath.Join(f.plugins, slug, pluginComposeFilename), socketFixture(t, used, fixture), 0o644)
	}
	if tc.override != "" {
		writeFixtureFile(t, filepath.Join(f.workdir, userComposeOverrideFile), socketFixture(t, used, tc.override), 0o644)
	}
	_, err := Build(f.workdir, BuildOptions{Force: true})
	return err
}

func TestSocketGuardFragments(t *testing.T) {
	setCLIVersion(t, "1.4.14")
	used := map[string]bool{}
	const (
		sockDev   = "/var/run/docker.sock"
		sockRun   = "/var/run"
		safeHost  = "tcp://proxy.invalid:2375"
		safeMount = "/srv/sockguard"
	)
	cases := []socketCase{
		// builds that must pass
		{"no socket", map[string]string{"sockclean": "clean.yml"}, "", nil, nil},
		{"remote tcp DOCKER_HOST is not a mount", map[string]string{"socktcp": "tcp_host.yml"}, "", nil, nil},
		{"config mount under the plugin dir resolves and passes", map[string]string{"sockvarok": "var_plugin_dir.yml"}, "", nil, nil},
		{"valid !reset and !override tags pass", map[string]string{"sockreset": "reset_tags.yml"}, "", nil, nil},
		{"variable source resolved to a safe path", map[string]string{"sockvarenv": "var_source_env.yml"}, "", map[string]string{"SOCKGUARD_SOURCE": safeMount}, nil},
		{"variable DOCKER_HOST resolved to tcp", map[string]string{"sockvarhost": "var_docker_host.yml"}, "", map[string]string{"SOCKGUARD_HOST": safeHost}, nil},
		{"variable device resolved to a safe path", map[string]string{"sockvardev": "var_device.yml"}, "", map[string]string{"SOCKGUARD_DEVICE": safeMount}, nil},
		{"core service in the override is not judged", map[string]string{"sockclean": "clean.yml"}, "override_admin.yml", nil, nil},
		{"ordinary named data volume stays allowed", map[string]string{"socksafevol": "safe_named_volume.yml"}, "", nil, nil},
		{"relative config mount inside the project stays allowed", map[string]string{"sockrelsafe": "relative_safe.yml"}, "", nil, nil},
		// socket mounts and DOCKER_HOST
		{"relative path climbing out to /var/run", map[string]string{"sockrelescape": "mount_relative_escape.yml"}, "", nil, []string{"sockrelescape"}},
		{"home-relative docker socket", map[string]string{"sockhome": "mount_home.yml"}, "", nil, []string{"sockhome"}},
		{"external volume on a plugin service", map[string]string{"sockext": "external_volume.yml"}, "", nil, []string{"sockext", "extvol", "cannot be proven"}},
		{"volume mapped with name:", map[string]string{"sockrenamed": "renamed_volume.yml"}, "", nil, []string{"sockrenamed", "renamedvol", "cannot be proven"}},
		{"short bind", map[string]string{"sockshort": "mount_short.yml"}, "", nil, []string{"sockshort"}},
		{"long bind", map[string]string{"socklong": "mount_long.yml"}, "", nil, []string{"socklong"}},
		{"interpolated default hides the socket", map[string]string{"sockvar": "mount_interpolated.yml"}, "", nil, []string{"sockvar"}},
		{"parent directory", map[string]string{"sockdir": "mount_parent_dir.yml"}, "", nil, []string{"sockdir"}},
		{"/private alias of the socket", map[string]string{"sockprivate": "mount_private_alias.yml"}, "", nil, []string{"sockprivate"}},
		{"named volume backed by the socket", map[string]string{"socknamed": "mount_named_volume.yml"}, "", nil, []string{"socknamed"}},
		{"named volume backed by the parent dir", map[string]string{"sockdevparent": "device_parent.yml"}, "", nil, []string{"sockdevparent", "parentvol"}},
		{"named volume backed by /private/var/run", map[string]string{"sockdevprivate": "device_private_alias.yml"}, "", nil, []string{"sockdevprivate"}},
		{"volume defined in one fragment, mounted by another", map[string]string{"sockvolowner": "shared_volume_owner.yml", "sockvolclient": "shared_volume_client.yml"}, "", nil, []string{"sockvolclient", "sharedsock"}},
		{"unix DOCKER_HOST list form", map[string]string{"sockenvlist": "docker_host_list.yml"}, "", nil, []string{"sockenvlist"}},
		{"unix DOCKER_HOST map form", map[string]string{"sockenvmap": "docker_host_map.yml"}, "", nil, []string{"sockenvmap"}},
		{"second service of the fragment", map[string]string{"sockpair": "second_service.yml"}, "", nil, []string{"sockpair", "socksecond"}},
		{"several findings in several services", map[string]string{"sockmulti": "multi.yml"}, "", nil, []string{"sockmulti"}},
		// indirection resolves, or fails closed
		{"DOCKER_HOST through a merge key", map[string]string{"sockmergeenv": "merge_env.yml"}, "", nil, []string{"sockmergeenv", "DOCKER_HOST"}},
		{"long volume through a merge key", map[string]string{"sockmergevol": "merge_long_volume.yml"}, "", nil, []string{"sockmergevol"}},
		{"variable source nobody sets", map[string]string{"sockvarunset": "var_source_unset.yml"}, "", nil, []string{"sockvarunset", "cannot be resolved"}},
		{"variable source set to the socket", map[string]string{"sockvarenv": "var_source_env.yml"}, "", map[string]string{"SOCKGUARD_SOURCE": sockDev}, []string{"sockvarenv"}},
		{"variable DOCKER_HOST nobody sets", map[string]string{"sockvarhost": "var_docker_host.yml"}, "", nil, []string{"sockvarhost"}},
		{"variable DOCKER_HOST set to a unix socket", map[string]string{"sockvarhost": "var_docker_host.yml"}, "", map[string]string{"SOCKGUARD_HOST": "unix://" + sockDev}, []string{"sockvarhost"}},
		{"variable device set to the parent dir", map[string]string{"sockvardev": "var_device.yml"}, "", map[string]string{"SOCKGUARD_DEVICE": sockRun}, []string{"sockvardev", "devvol"}},
		{"variable device nobody sets", map[string]string{"sockvardev": "var_device.yml"}, "", nil, []string{"sockvardev", "cannot be resolved"}},
		{"env_file is not followed", map[string]string{"sockenvfile": "env_file.yml"}, "", nil, []string{"sockenvfile", "env_file"}},
		{"extends is not followed", map[string]string{"sockextends": "extends_file.yml"}, "", nil, []string{"sockextends", "extends"}},
		{"volumes_from is not followed", map[string]string{"sockvolsfrom": "volumes_from.yml"}, "", nil, []string{"sockvolsfrom", "volumes_from"}},
		{"include is not followed", map[string]string{"sockinclude": "include.yml"}, "", nil, []string{"sockinclude", "include"}},
		{"unparseable fragment is refused", map[string]string{"sockbroken": "invalid.yml"}, "", nil, []string{"sockbroken", "cannot be checked"}},
		{"environment as a string is refused", map[string]string{"sockbadenv": "malformed_env.yml"}, "", nil, []string{"sockbadenv"}},
		{"volumes as a string is refused", map[string]string{"sockbadvols": "malformed_volumes.yml"}, "", nil, []string{"sockbadvols"}},
		{"numeric volume entry is refused", map[string]string{"sockbadentry": "malformed_volume_entry.yml"}, "", nil, []string{"sockbadentry"}},
		{"services as a string is refused", map[string]string{"sockbadservices": "malformed_services.yml"}, "", nil, []string{"sockbadservices"}},
		{"list key variable resolving to DOCKER_HOST", map[string]string{"sockkeyvar": "environment_key_var.yml"}, "", map[string]string{"SOCKGUARD_KEY": "DOCKER_HOST"}, []string{"sockkeyvar", "DOCKER_HOST"}},
		{"list key variable resolving to a harmless key", map[string]string{"sockkeyvar": "environment_key_var.yml"}, "", map[string]string{"SOCKGUARD_KEY": "HARMLESS"}, nil},
		{"list key variable that cannot be resolved", map[string]string{"sockkeyvar": "environment_key_var.yml"}, "", nil, []string{"sockkeyvar", "cannot be resolved"}},
		{"map keys are not interpolated", map[string]string{"sockkeymap": "environment_key_map.yml"}, "", map[string]string{"SOCKGUARD_KEY": "DOCKER_HOST"}, nil},
		// exceptions never widen: they waive a confirmed socket on the plugin's own service, not what the guard cannot prove
		{"excepted plugin: unresolved mount source", map[string]string{"forgejo": "var_source_unset.yml"}, "", nil, []string{"forgejo", "sockvarunset", "cannot be resolved"}},
		{"excepted plugin: unresolved DOCKER_HOST", map[string]string{"forgejo": "var_docker_host.yml"}, "", nil, []string{"forgejo", "sockvarhost"}},
		{"excepted plugin: unresolved volume device", map[string]string{"forgejo": "var_device.yml"}, "", nil, []string{"forgejo", "sockvardev", "cannot be proven"}},
		{"excepted plugin: external volume", map[string]string{"forgejo": "external_volume.yml"}, "", nil, []string{"forgejo", "sockext", "cannot be proven"}},
		{"excepted plugin: renamed volume that also names a socket device", map[string]string{"forgejo": "combo_renamed.yml"}, "", nil, []string{"forgejo", "sockcombo", "cannot be proven"}},
		{"excepted plugin: external volume that also names a socket device", map[string]string{"monitoring": "combo_external.yml"}, "", nil, []string{"monitoring", "sockcomboext", "cannot be proven"}},
		{"excepted plugin: renamed volume", map[string]string{"monitoring": "renamed_volume.yml"}, "", nil, []string{"monitoring", "sockrenamed", "cannot be proven"}},
		{"excepted plugin: env_file", map[string]string{"forgejo": "env_file.yml"}, "", nil, []string{"forgejo", "sockenvfile", "env_file"}},
		{"excepted plugin: extends", map[string]string{"forgejo": "extends_file.yml"}, "", nil, []string{"forgejo", "sockextends", "extends"}},
		{"excepted plugin: volumes_from", map[string]string{"monitoring": "volumes_from.yml"}, "", nil, []string{"monitoring", "sockvolsfrom", "volumes_from"}},
		{"an excepted plugin cannot hold the proxy service", map[string]string{"forgejo": "proxy_nginx.yml"}, "", nil, []string{"forgejo", "nginx"}},
		{"ambiguous ownership grants no exception", map[string]string{"forgejo": "ambiguous_a.yml", "other": "ambiguous_b.yml"}, "", nil, []string{"sockdup"}},
		{"override adds the socket to nginx", map[string]string{"sockclean": "clean.yml"}, "override_nginx.yml", nil, []string{"nginx", "override"}},
		{"override adds the socket to an excepted plugin's service", map[string]string{"forgejo": "clean.yml"}, "override_plugin.yml", nil, []string{"forgejo", "sockclean"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := buildSocketCase(t, used, tc)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("build refused a safe case: %v", err)
				}
				return
			}
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E130" {
				t.Fatalf("want E130, got %v", err)
			}
			for _, s := range tc.want {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("E130 %q lacks %q", err, s)
				}
			}
		})
	}

	t.Run("excepted plugin warns once per plugin across builds, findings and services", func(t *testing.T) {
		resetWarnOnce()
		f := newPlanFixture(t, "dev-minimal")
		writeFixtureFile(t, filepath.Join(f.plugins, "forgejo", pluginComposeFilename), socketFixture(t, used, "multi.yml"), 0o644)
		for i := 0; i < 2; i++ {
			if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
				t.Fatalf("excepted plugin refused on build %d: %v", i+1, err)
			}
		}
		if n := warnedCount(); n != 1 {
			t.Errorf("want exactly one warning for forgejo, got %d", n)
		}
		writeFixtureFile(t, filepath.Join(f.plugins, "monitoring", pluginComposeFilename), socketFixture(t, used, "mount_short.yml"), 0o644)
		if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
			t.Fatal(err)
		}
		if n := warnedCount(); n != 2 {
			t.Errorf("want one warning per excepted plugin (2), got %d", n)
		}
	})

	t.Run("an exception covers its own plugin only", func(t *testing.T) {
		err := buildSocketCase(t, used, socketCase{plugins: map[string]string{"forgejo": "mount_short.yml", "other": "docker_host_map.yml"}})
		if err == nil || !strings.Contains(err.Error(), `"other"`) {
			t.Fatalf("want E130 naming other, got %v", err)
		}
	})

	t.Run("symlinked relative bind and device", func(t *testing.T) {
		safe := t.TempDir()
		for _, fx := range []struct{ fixture, slug string }{{"mount_symlink.yml", "socklink"}, {"device_symlink.yml", "socklinkdev"}, {"bare_long_bind.yml", "sockbare"}, {"device_symlink_bare.yml", "sockbaredev"}} {
			for name, tc := range map[string]struct {
				target  string
				refused bool
			}{
				"to /var/run":   {"/var/run", true},
				"to a safe dir": {safe, false},
				"dangling":      {filepath.Join(safe, "gone"), true},
			} {
				t.Run(fx.slug+" "+name, func(t *testing.T) {
					f := newPlanFixture(t, "dev-minimal")
					writeFixtureFile(t, filepath.Join(f.plugins, fx.slug, pluginComposeFilename), socketFixture(t, used, fx.fixture), 0o644)
					if err := os.Symlink(tc.target, filepath.Join(f.workdir, "run")); err != nil {
						t.Fatal(err)
					}
					_, err := Build(f.workdir, BuildOptions{Force: true})
					if tc.refused != (err != nil) || tc.refused && !strings.Contains(err.Error(), fx.slug) {
						t.Fatalf("refused=%v, got %v", tc.refused, err)
					}
				})
			}
		}
	})

	t.Run("effective env: forwarding, nested dotenv, stale artifact", func(t *testing.T) {
		const unix, tcp = "unix:///var/run/docker.sock", "tcp://safe.invalid:2375"
		for _, tc := range []struct {
			name, fixture, slug, dotenv, stale string
			host                               *string // process DOCKER_HOST; nil = unset
			refused                            bool
		}{
			{"forwarded list entry, effective unix", "docker_host_inherit_list.yml", "sockinheritlist", "", "", ptr(unix), true},
			{"forwarded list entry, effective tcp", "docker_host_inherit_list.yml", "sockinheritlist", "", "", ptr(tcp), false},
			{"forwarded list entry, unset", "docker_host_inherit_list.yml", "sockinheritlist", "", "", nil, false},
			{"forwarded null entry, effective unix", "docker_host_inherit_null.yml", "sockinheritnull", "", "", ptr(unix), true},
			{"forwarded null entry, effective tcp", "docker_host_inherit_null.yml", "sockinheritnull", "", "", ptr(tcp), false},
			{"forwarded null entry, unset", "docker_host_inherit_null.yml", "sockinheritnull", "", "", nil, false},
			{"forwarded entry, unix from the project .env", "docker_host_inherit_list.yml", "sockinheritlist", "DOCKER_HOST=" + unix + "\n", "", nil, true},
			{"nested dotenv value resolves to unix", "docker_host_nested.yml", "socknested", "SOCKGUARD_BASE=" + unix + "\nSOCKGUARD_NESTED=${SOCKGUARD_BASE}\n", "", nil, true},
			{"nested dotenv value resolves to tcp", "docker_host_nested.yml", "socknested", "SOCKGUARD_BASE=" + tcp + "\nSOCKGUARD_NESTED=${SOCKGUARD_BASE}\n", "", nil, false},
			{"single-quoted dotenv value keeps a dollar and fails closed", "docker_host_nested.yml", "socknested", "SOCKGUARD_BASE=" + tcp + "\nSOCKGUARD_NESTED='${SOCKGUARD_BASE}'\n", "", nil, true},
			{"stale compose.env tcp does not hide the project .env unix", "var_docker_host.yml", "sockvarhost", "SOCKGUARD_HOST=" + unix + "\n", "SOCKGUARD_HOST=" + tcp + "\n", nil, true},
			{"stale compose.env unix does not refuse a safe project .env", "var_docker_host.yml", "sockvarhost", "SOCKGUARD_HOST=" + tcp + "\n", "SOCKGUARD_HOST=" + unix + "\n", nil, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newPlanFixture(t, "dev-minimal")
				if tc.host == nil {
					old, had := os.LookupEnv("DOCKER_HOST")
					_ = os.Unsetenv("DOCKER_HOST")
					t.Cleanup(func() {
						if had {
							_ = os.Setenv("DOCKER_HOST", old)
						}
					})
				} else {
					t.Setenv("DOCKER_HOST", *tc.host)
				}
				if tc.dotenv != "" {
					base, _ := os.ReadFile(filepath.Join(f.workdir, ".env"))
					writeFixtureFile(t, filepath.Join(f.workdir, ".env"), string(base)+tc.dotenv, 0o600)
				}
				if tc.stale != "" {
					writeFixtureFile(t, filepath.Join(f.workdir, ".nself", "compose.env"), tc.stale, 0o600)
				}
				writeFixtureFile(t, filepath.Join(f.plugins, tc.slug, pluginComposeFilename), socketFixture(t, used, tc.fixture), 0o644)
				_, err := Build(f.workdir, BuildOptions{Force: true})
				if tc.refused != (err != nil) || tc.refused && !strings.Contains(err.Error(), tc.slug) {
					t.Fatalf("refused=%v, got %v", tc.refused, err)
				}
			})
		}
	})

	t.Run("a disabled plugin is not checked", func(t *testing.T) {
		f := newPlanFixture(t, "dev-minimal")
		writeFixtureFile(t, filepath.Join(f.plugins, "sockoff", pluginComposeFilename), socketFixture(t, used, "mount_short.yml"), 0o644)
		writeFixtureFile(t, filepath.Join(f.plugins, "sockoff", ".disabled"), "", 0o644)
		if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
			t.Fatalf("disabled plugin refused: %v", err)
		}
	})

	t.Run("generated stack", func(t *testing.T) {
		none := func(string) (string, bool) { return "", false }
		check := func(stack string) error {
			rep, err := AnalyzeSocket(t.TempDir(), t.TempDir(), none, []byte(stack))
			if err != nil {
				return err
			}
			return rep.Refuse()
		}
		const sock = `["/var/run/docker.sock:/var/run/docker.sock:ro"]`
		for _, proxy := range []string{"nginx", "traefik"} {
			if err := check("services:\n  " + proxy + ":\n    image: x\n    volumes: " + sock + "\n"); err == nil || !strings.Contains(err.Error(), `"`+proxy+`"`) {
				t.Errorf("%s with the socket must be refused, got %v", proxy, err)
			}
		}
		if err := check("services:\n  traefik:\n    environment: {DOCKER_HOST: \"unix:///var/run/docker.sock\"}\n"); err == nil {
			t.Error("traefik with a unix DOCKER_HOST must be refused")
		}
		if err := check("services:\n  nself-admin:\n    volumes: " + sock + "\n  ping-api:\n    volumes: " + sock + "\n  postgres:\n    image: x\n"); err != nil {
			t.Errorf("admin, custom and other core services are not judged: %v", err)
		}
		if err := check("services: [unclosed"); err == nil {
			t.Error("an unreadable stack must not pass")
		}
	})

	t.Run("socket path and variable rules", func(t *testing.T) {
		for p, want := range map[string]bool{
			"/var/run/docker.sock": true, "/var/run/docker.sock/": true, "/run/docker.sock": true, "/var/run": true, "/run": true, "/var": true, "/": true,
			"/private/var/run": true, "/private/var/run/docker.sock": true, "/private": true, "~/.docker/run/docker.sock": true,
			"/srv/data": false, "./data": false, "data": false, "": false, "/privatex": false,
			"/var/lib/docker": false, "/var/run/podman/podman.sock": false, // other daemon policy is out of scope
		} {
			if got := SocketPathExposed(p); got != want {
				t.Errorf("SocketPathExposed(%q) = %v, want %v", p, got, want)
			}
		}
		env := func(k string) (string, bool) { v, ok := map[string]string{"A": "/a", "E": ""}[k]; return v, ok }
		for in, want := range map[string]string{"${A}/x": "/a/x", "${U:-/d}": "/d", "${E:-/d}": "/d", "${A:-/d}": "/a", "$$A": "$A", "$A": "/a", "x${A}y": "x/ay", "${U:-${A}}": ""} {
			if got, ok := expandSocket(in, env); in == "${U:-${A}}" && ok || in != "${U:-${A}}" && (!ok || got != want) {
				t.Errorf("expandSocket(%q) = %q, %v; want %q", in, got, ok, want)
			}
		}
		for _, in := range []string{"${U}", "${E-/d}", "$U", "a$", "${A:?x}", "${A:+x}", "${A"} {
			if _, ok := expandSocket(in, env); ok {
				t.Errorf("expandSocket(%q) must not resolve", in)
			}
		}
	})

	t.Run("override-only services are not core", func(t *testing.T) {
		wd := t.TempDir()
		writeFixtureFile(t, filepath.Join(wd, userComposeOverrideFile), socketFixture(t, used, "override_plugin.yml"), 0o644)
		none := func(string) (string, bool) { return "", false }
		rep, err := AnalyzeSocket(wd, t.TempDir(), none, []byte("services:\n  nginx:\n    volumes: [\"./a:/a\"]\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !rep.Core["nginx"] || rep.Core["sockclean"] {
			t.Fatalf("core=%v", rep.Core)
		}
	})

	t.Run("env layering: process env, base, then files in order", func(t *testing.T) {
		dir := t.TempDir()
		a, b := filepath.Join(dir, "a.env"), filepath.Join(dir, "b.env")
		writeFixtureFile(t, a, "L=from-a\nM=${L}-m\nN=a\n", 0o600)
		writeFixtureFile(t, b, "N=b\nP=b\n", 0o600)
		t.Setenv("SOCKGUARD_LAYER_PROC", "proc")
		env := NewSocketEnv(map[string]string{"N": "base-n", "SOCKGUARD_LAYER_PROC": "base"}, a, b, filepath.Join(dir, "missing.env"))
		for k, want := range map[string]string{"L": "from-a", "M": "from-a-m", "N": "base-n", "P": "b", "SOCKGUARD_LAYER_PROC": "proc"} {
			if got, ok := env(k); !ok || got != want {
				t.Errorf("%s = %q, %v; want %q", k, got, ok, want)
			}
		}
		if _, ok := env("SOCKGUARD_NOPE"); ok {
			t.Error("an unset variable resolved")
		}
	})

	t.Run("dependent dotenv values need the exact context", func(t *testing.T) {
		const unix, tcp = "unix:///var/run/docker.sock", "tcp://safe.invalid:2375"
		unset := func(k string) {
			old, had := os.LookupEnv(k)
			_ = os.Unsetenv(k)
			t.Cleanup(func() {
				if had {
					_ = os.Setenv(k, old)
				}
			})
		}
		write := func(name, body string) string {
			p := filepath.Join(t.TempDir(), name)
			writeFixtureFile(t, p, body, 0o600)
			return p
		}
		check := func(label string, env SocketEnv, name, want string, wantOK bool) {
			t.Helper()
			if got, ok := expandSocket("${"+name+"}", env); ok != wantOK || ok && got != want {
				t.Errorf("%s: ${%s} = %q, %v; want %q, %v", label, name, got, ok, want, wantOK)
			}
		}
		for _, k := range []string{"SOCKGUARD_H", "SOCKGUARD_HOST", "SOCKGUARD_B", "SOCKGUARD_A", "SOCKGUARD_X", "SOCKGUARD_Y", "SOCKGUARD_BASE", "SOCKGUARD_NESTED", "NSELF_PLUGIN_DIR"} {
			unset(k)
		}
		dotenv := write(".env", "SOCKGUARD_H="+tcp+"\nSOCKGUARD_HOST=${SOCKGUARD_H}\n")
		t.Setenv("SOCKGUARD_H", unix) // the shell wins over the file in Compose
		check("process differs from the file", NewSocketEnv(nil, dotenv), "SOCKGUARD_HOST", "", false)
		check("the shell value itself", NewSocketEnv(nil, dotenv), "SOCKGUARD_H", unix, true)
		t.Setenv("SOCKGUARD_H", tcp)
		check("process equals the file", NewSocketEnv(nil, dotenv), "SOCKGUARD_HOST", tcp, true)
		_ = os.Unsetenv("SOCKGUARD_H")
		check("process unset: isolated nested expansion is exact", NewSocketEnv(nil, dotenv), "SOCKGUARD_HOST", tcp, true)
		check("cross-file dependency", NewSocketEnv(nil, write("a.env", "SOCKGUARD_B="+tcp+"\n"), write("b.env", "SOCKGUARD_A=${SOCKGUARD_B}\n")), "SOCKGUARD_A", "", false)
		check("computed value redefines a referenced name", NewSocketEnv(map[string]string{"SOCKGUARD_X": "2"}, write("c.env", "SOCKGUARD_X=1\nSOCKGUARD_Y=${SOCKGUARD_X}\n")), "SOCKGUARD_Y", "", false)
		check("forward reference", NewSocketEnv(nil, write("d.env", "SOCKGUARD_Y=${SOCKGUARD_X}\nSOCKGUARD_X=1\n")), "SOCKGUARD_Y", "", false)
		check("name defined nowhere in the files", NewSocketEnv(nil, write("e.env", "SOCKGUARD_Y=${SOCKGUARD_X}\n")), "SOCKGUARD_Y", "", false)
		check("computed plugin variable", NewSocketEnv(map[string]string{"NSELF_PLUGIN_DIR": "/p"}, write("f.env", "SOCKGUARD_Y=literal\n")), "NSELF_PLUGIN_DIR", "/p", true)
		check("plain literal", NewSocketEnv(nil, write("g.env", "SOCKGUARD_Y=literal\n")), "SOCKGUARD_Y", "literal", true)

		chain := write("j.env", "SOCKGUARD_H="+tcp+"\nSOCKGUARD_A=${SOCKGUARD_H}\nSOCKGUARD_HOST=${SOCKGUARD_A}\n")
		t.Setenv("SOCKGUARD_H", unix)
		check("transitive: process value differs two links down", NewSocketEnv(nil, chain), "SOCKGUARD_HOST", "", false)
		check("transitive: the middle link", NewSocketEnv(nil, chain), "SOCKGUARD_A", "", false)
		_ = os.Unsetenv("SOCKGUARD_H")
		check("transitive chain is exact when the process holds nothing", NewSocketEnv(nil, chain), "SOCKGUARD_HOST", tcp, true)
		check("a cycle is unprovable, not a loop", NewSocketEnv(nil, write("k.env", "SOCKGUARD_A=${SOCKGUARD_B}\nSOCKGUARD_B=${SOCKGUARD_A}\n")), "SOCKGUARD_A", "", false)

		// End to end: the fragment's DOCKER_HOST=${SOCKGUARD_NESTED} against the process-H repro.
		pluginDir := t.TempDir()
		writeFixtureFile(t, filepath.Join(pluginDir, "socknested", pluginComposeFilename), socketFixture(t, used, "docker_host_nested.yml"), 0o644)
		env := NewSocketEnv(nil, write("h.env", "SOCKGUARD_BASE="+tcp+"\nSOCKGUARD_NESTED=${SOCKGUARD_BASE}\n"))
		refuse := func(env SocketEnv) error {
			rep, err := AnalyzeSocket(t.TempDir(), pluginDir, env, []byte("services:\n  nginx:\n    image: x\n"))
			if err != nil {
				return err
			}
			return rep.Refuse()
		}
		if err := refuse(env); err != nil {
			t.Fatalf("exact isolated nesting to tcp must pass: %v", err)
		}
		t.Setenv("SOCKGUARD_BASE", unix)
		if err := refuse(NewSocketEnv(nil, write("i.env", "SOCKGUARD_BASE="+tcp+"\nSOCKGUARD_NESTED=${SOCKGUARD_BASE}\n"))); err == nil || !strings.Contains(err.Error(), "socknested") {
			t.Fatalf("a process value that differs from the file must make the nested DOCKER_HOST unprovable: %v", err)
		}
	})

	t.Run("the guard judges the compose.env this build will write, not stale .env fields", func(t *testing.T) {
		f := newPlanFixture(t, "dev-minimal")
		old, _ := os.ReadFile(filepath.Join(f.workdir, ".env"))
		writeFixtureFile(t, filepath.Join(f.workdir, ".env"), string(old)+"DOCKER_NETWORK=tcp://stale.invalid:2375\n", 0o600)
		defer SnapshotEnv()()
		cfg, err := config.Load(f.workdir)
		if err != nil {
			t.Fatal(err)
		}
		_ = os.Unsetenv("DOCKER_NETWORK") // config.Load exported the stale field; the process holds nothing for compose.env to lose to
		core := []byte("services:\n  nginx:\n    image: x\n")
		for _, tc := range []struct {
			fixture, slug, network string
			refused                bool
		}{
			{"docker_host_future.yml", "sockfuturehost", "unix:///var/run/docker.sock", true}, // the .env field says tcp; the written value is unix
			{"mount_future.yml", "sockfuturemount", "/var/run", true},                         // ... and a path that holds the socket
			{"docker_host_future.yml", "sockfuturehost", "tcp://exact.invalid:2375", false},
			{"mount_future.yml", "sockfuturemount", "golden_network", false}, // a plain named volume, exactly represented
		} {
			cfg.DockerNetwork = tc.network
			st := &buildState{workdir: f.workdir, pluginDir: f.plugins, cfg: cfg}
			if got := st.futureComposeEnv()["DOCKER_NETWORK"]; got != tc.network {
				t.Fatalf("future compose.env DOCKER_NETWORK = %q, want %q", got, tc.network)
			}
			dir := t.TempDir()
			writeFixtureFile(t, filepath.Join(dir, tc.slug, pluginComposeFilename), socketFixture(t, used, tc.fixture), 0o644)
			rep, err := AnalyzeSocket(f.workdir, dir, NewSocketEnv(st.futureComposeEnv(), filepath.Join(f.workdir, ".env")), core)
			if err == nil {
				err = rep.Refuse()
			}
			if tc.refused != (err != nil) || tc.refused && !strings.Contains(err.Error(), tc.slug) {
				t.Errorf("network %q: refused=%v, got %v", tc.network, tc.refused, err)
			}
		}
		// the stale .env value alone would have passed the first two
		stale := NewSocketEnv(nil, filepath.Join(f.workdir, ".env"))
		if v, _ := stale("DOCKER_NETWORK"); v != "tcp://stale.invalid:2375" {
			t.Errorf("fixture precondition: stale DOCKER_NETWORK = %q", v)
		}
	})

	t.Run("every fixture is exercised", func(t *testing.T) {
		entries, err := os.ReadDir(filepath.Join("testdata", "socket"))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !used[e.Name()] {
				t.Errorf("fixture %s is never used", e.Name())
			}
		}
	})
}

func TestSocketGuardExceptionExpiry(t *testing.T) {
	t.Run("table entries all expire", func(t *testing.T) {
		for slug, ex := range socketExceptions {
			if ex.Slug != slug || ex.Ticket == "" || !activeAt(slug, "0.0.0") || activeAt(slug, ex.RemovalAt) {
				t.Errorf("%s: removal_at %q ticket %q slug %q is not a usable, expiring entry", slug, ex.RemovalAt, ex.Ticket, ex.Slug)
			}
		}
		if len(socketExceptions) != 2 || socketExceptions["forgejo"].Ticket != "P7-TRUST-21" || socketExceptions["monitoring"].Ticket != "P7-PLUG-58" {
			t.Error("the table is forgejo (P7-TRUST-21) and monitoring (P7-PLUG-58) only")
		}
		if socketExceptions["forgejo"].RemovalAt != "1.5.0" || socketExceptions["monitoring"].RemovalAt != "1.5.0" {
			t.Error("both exceptions end at v1.5.0")
		}
	})

	cases := []struct {
		version string
		active  bool
	}{
		{"1.4.14", true}, {"1.4.99", true}, {"v1.4.0", true},
		{"1.5.0", false}, {"1.5.0-rc.1", false}, {"1.5.1", false}, {"2.0.0", false},
		{"dev", false}, {"", false}, {"1.5", false}, {"1.x.0", false}, // unknown version: fail closed
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
		used := map[string]bool{}
		for _, slug := range []string{"forgejo", "monitoring"} {
			tc := socketCase{plugins: map[string]string{slug: "mount_short.yml"}}
			setCLIVersion(t, "1.4.14")
			if err := buildSocketCase(t, used, tc); err != nil {
				t.Fatalf("%s refused before removal: %v", slug, err)
			}
			setCLIVersion(t, socketExceptions[slug].RemovalAt)
			err := buildSocketCase(t, used, tc)
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E130" || !strings.Contains(err.Error(), slug) {
				t.Fatalf("%s at its removal version: want E130, got %v", slug, err)
			}
		}
	})

	t.Run("a proxy or an ambiguous owner never inherits an exception", func(t *testing.T) {
		setCLIVersion(t, "1.4.14")
		use := []SocketUse{{Origin: "plugin:forgejo", Detail: "mounts the Docker socket"}}
		for name, rep := range map[string]*SocketReport{
			"proxy":     {Uses: map[string][]SocketUse{"nginx": use}, Owners: map[string][]string{"nginx": {"forgejo"}}},
			"ambiguous": {Uses: map[string][]SocketUse{"svc": use}, Owners: map[string][]string{"svc": {"forgejo", "monitoring"}}},
			"unproven":  {Uses: map[string][]SocketUse{"svc": {{Origin: "plugin:forgejo", Detail: "uses env_file", Unproven: true}}}, Owners: map[string][]string{"svc": {"forgejo"}}},
			"foreign":   {Uses: map[string][]SocketUse{"svc": {{Origin: "override", Detail: "mounts the Docker socket"}}}, Owners: map[string][]string{"svc": {"forgejo"}}},
		} {
			if err := rep.Refuse(); err == nil {
				t.Errorf("%s: an exception was inherited", name)
			}
		}
		resetWarnOnce()
		ok := &SocketReport{Uses: map[string][]SocketUse{"a": use, "b": use}, Owners: map[string][]string{"a": {"forgejo"}, "b": {"forgejo"}}}
		if err := ok.Refuse(); err != nil || warnedCount() != 1 {
			t.Errorf("own exposure in two services: err=%v warnings=%d, want nil and 1", err, warnedCount())
		}
	})
}
