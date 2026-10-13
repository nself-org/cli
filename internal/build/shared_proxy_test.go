package build

// shared_proxy_test.go — P7-ADOPT-03: the shared host proxy build (EPIC ADOPT D2).
//
// Purpose: prove the owner (NGINX_MODE=shared) and a fronted project nested
// under it render container-name upstreams, prefixed files and a registration,
// and that cross-project collisions fail closed before anything is written.
// Inputs: hermetic projects built with Build under one temp root: owner at
// <root>/stack, fronted projects at <root>/stack/backend and <root>/stack/other.
// Outputs: goldens under testdata/shared_proxy/ (UPDATE_GOLDEN=1 rewrites them).
// Constraints: secrets are fixture values; the plugin dir is shared by every
// project of one test, as it is on a real host.

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/sharedproxy"
)

// sharedFx is one owner stack with room for fronted projects under it.
type sharedFx struct {
	root, home, plugins, owner string
}

func newSharedFx(t *testing.T) *sharedFx {
	t.Helper()
	isolateEnv(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	x := &sharedFx{root: root, home: filepath.Join(root, "home"), plugins: filepath.Join(root, "plugins"), owner: filepath.Join(root, "stack")}
	for _, d := range []string{x.home, x.plugins, x.owner} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return x
}

// env renders a project's .env: its name, domain and extra lines.
func (x *sharedFx) env(project, domain string, extra ...string) string {
	lines := append([]string{"PROJECT_NAME=" + project, "ENV=dev", "BASE_DOMAIN=" + domain, "SSL_MODE=none", "MINIO_ENABLED=false"}, extra...)
	return strings.Join(lines, "\n") + "\n" + fixtureSecrets
}

// build writes dir/.env and builds dir, with a clean build environment each time.
func (x *sharedFx) build(t *testing.T, dir, env string) error {
	t.Helper()
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok && isBuildEnvKey(k) {
			_ = os.Unsetenv(k)
		}
	}
	t.Setenv("HOME", x.home)
	t.Setenv("USERPROFILE", x.home)
	t.Setenv("NSELF_PLUGIN_DIR", x.plugins)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(dir, ".env"), env, 0o600)
	_, err := Build(dir, BuildOptions{Force: true})
	return err
}

func (x *sharedFx) mustBuild(t *testing.T, dir, env string) {
	t.Helper()
	if err := x.build(t, dir, env); err != nil {
		t.Fatalf("build %s: %v", dir, err)
	}
}

func (x *sharedFx) backend() string { return filepath.Join(x.owner, "backend") }
func (x *sharedFx) other() string   { return filepath.Join(x.owner, "other") }
func (x *sharedFx) sites() string   { return filepath.Join(x.owner, "nginx", "sites") }
func (x *sharedFx) shared() string  { return filepath.Join(x.owner, "nginx", "shared") }

// ownerEnv / frontedEnv are the standard projects of these tests.
func (x *sharedFx) ownerEnv(extra ...string) string {
	return x.env("owna", "a.test", append([]string{"NGINX_MODE=shared"}, extra...)...)
}
func (x *sharedFx) frontedEnv(project, domain string, extra ...string) string {
	return x.env(project, domain, append([]string{"NGINX_FRONTED_BY=stack"}, extra...)...)
}

// plugin installs a plugin with a compose fragment and an nginx snippet whose
// upstream is the bare host name.
func (x *sharedFx) plugin(t *testing.T, name string, port int) {
	t.Helper()
	dir := filepath.Join(x.plugins, name)
	writeFixtureFile(t, filepath.Join(dir, "plugin.json"), fmt.Sprintf(`{"name":%q,"port":%d,"language":"go"}`, name, port), 0o644)
	writeFixtureFile(t, filepath.Join(dir, pluginComposeFilename), fmt.Sprintf("services:\n  %s:\n    image: nself/%s:latest\n    ports:\n      - \"%d:%d\"\n", name, name, port, port), 0o644)
	writeFixtureFile(t, filepath.Join(dir, "Dockerfile"), "FROM scratch\n", 0o644)
	writeFixtureFile(t, filepath.Join(dir, "nginx", "route.conf"),
		fmt.Sprintf("server {\n  listen 80;\n  server_name %s.${BASE_DOMAIN};\n  location / {\n    proxy_pass http://%s:%d;\n  }\n}\n", name, name, port), 0o644)
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// treeDigest hashes every file below dirs (path, mode, bytes), so a refusal can
// prove nothing changed. A missing dir hashes as empty.
func treeDigest(t *testing.T, dirs ...string) string {
	t.Helper()
	h := sha256.New()
	for _, d := range dirs {
		var paths []string
		_ = filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err == nil {
				paths = append(paths, p)
			}
			return nil
		})
		sort.Strings(paths)
		for _, p := range paths {
			fi, err := os.Lstat(p)
			if err != nil {
				continue
			}
			fmt.Fprintf(h, "%s %v\n", p, fi.Mode())
			if fi.Mode().IsRegular() {
				b, _ := os.ReadFile(p)
				h.Write(b)
			}
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// golden compares got with testdata/shared_proxy/<name>, the temp root as <ROOT>.
func (x *sharedFx) golden(t *testing.T, name, got string) {
	t.Helper()
	got = normalizeFixtureBytes("", got, x.root)
	path := filepath.Join("testdata", "shared_proxy", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		writeFixtureFile(t, path, got, 0o644)
	}
	if want := readText(t, path); want != got {
		t.Errorf("%s differs from its golden:\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

// wantCode fails unless err is a CLIError with code and every needle in its text.
func wantCode(t *testing.T, err error, code string, needles ...string) {
	t.Helper()
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
	for _, n := range needles {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("error %q lacks %q", err.Error(), n)
		}
	}
}

func TestSharedProxyOwner(t *testing.T) {
	x := newSharedFx(t)
	x.plugin(t, "notify", 3709)
	x.mustBuild(t, x.owner, x.ownerEnv())

	hasura := readText(t, filepath.Join(x.sites(), "hasura.conf"))
	if !strings.Contains(hasura, "owna_hasura:8080") || strings.Contains(hasura, "http://hasura:8080") {
		t.Errorf("owner hasura.conf must use the container name:\n%s", hasura)
	}
	x.golden(t, "owner/hasura.conf", hasura)
	x.golden(t, "owner/auth.conf", readText(t, filepath.Join(x.sites(), "auth.conf")))
	// A plugin's upstream is left as written for the owner.
	if plugin := readText(t, filepath.Join(x.sites(), "notify-route.conf")); !strings.Contains(plugin, "http://notify:3709") {
		t.Errorf("owner plugin upstream must stay bare:\n%s", plugin)
	}
	routes := readText(t, filepath.Join(x.owner, ".nself", "generated", "routes.json"))
	if !strings.Contains(routes, `"owna_hasura"`) || strings.Contains(routes, `"host": "hasura"`) {
		t.Errorf("routes.json must carry container-name hosts")
	}
	x.golden(t, "owner/_owner.json", readText(t, filepath.Join(x.shared(), sharedproxy.OwnerFile)))

	set, err := sharedproxy.Load(x.shared(), x.owner)
	if err != nil || set.Owner == nil || set.Owner.Project != "owna" || len(set.Owner.Files) != 0 {
		t.Fatalf("owner registration: %+v %v", set.Owner, err)
	}
}

func TestSharedProxyFronted(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	ownerSites := treeDigest(t, x.sites())
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test", "MAILPIT_ENABLED=true"))
	if _, err := os.Stat(filepath.Join(x.sites(), "bproj.mail.conf")); err != nil {
		t.Fatalf("mail route missing: %v", err)
	}

	for _, f := range []string{"hasura", "auth"} {
		conf := readText(t, filepath.Join(x.sites(), "bproj."+f+".conf"))
		if !strings.HasPrefix(conf, "# Generated by nself build (project bproj)\n") {
			t.Errorf("bproj.%s.conf marker:\n%s", f, conf)
		}
		if strings.Contains(conf, "http://"+f+":") || !strings.Contains(conf, "bproj_"+f) {
			t.Errorf("bproj.%s.conf must use the container name:\n%s", f, conf)
		}
	}
	x.golden(t, "fronted/bproj.hasura.conf", readText(t, filepath.Join(x.sites(), "bproj.hasura.conf")))
	reg := readText(t, filepath.Join(x.shared(), "bproj.json"))
	x.golden(t, "fronted/bproj.json", reg)

	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "schemas", "shared-proxy.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	var instance any
	if err := json.Unmarshal([]byte(reg), &instance); err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(instance); err != nil {
		t.Fatalf("registration violates its schema: %v", err)
	}
	set, err := sharedproxy.Load(x.shared(), x.owner)
	if err != nil {
		t.Fatal(err)
	}
	b := set.Find("bproj")
	if b == nil || b.Network != "bproj_network" || len(b.Files) < 2 || !inSorted(b.ServerNames, "api.b.test") {
		t.Fatalf("registration content: %+v", b)
	}
	// The owner's own files were not touched by B, and B's survive the owner's rebuild.
	if got := treeDigest(t, x.sites()); got == ownerSites {
		t.Fatal("B wrote nothing into the owner's sites dir")
	}
	before := readText(t, filepath.Join(x.sites(), "bproj.hasura.conf"))
	x.mustBuild(t, x.owner, x.ownerEnv())
	if got := readText(t, filepath.Join(x.sites(), "bproj.hasura.conf")); got != before {
		t.Fatal("the owner's rebuild changed or removed a registered file")
	}
	for _, f := range b.Files {
		if _, err := os.Stat(filepath.Join(x.owner, filepath.FromSlash(f))); err != nil {
			t.Errorf("registered file %s is gone after the owner's rebuild: %v", f, err)
		}
	}
	// B's rebuild after a domain change removes exactly its own stale files.
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "c.test"))
	if _, err := os.Stat(filepath.Join(x.sites(), "bproj.mail.conf")); !os.IsNotExist(err) {
		t.Fatalf("bproj.mail.conf is stale and must be removed: %v", err)
	}
	set, _ = sharedproxy.Load(x.shared(), x.owner)
	if b = set.Find("bproj"); b == nil || !inSorted(b.ServerNames, "api.c.test") || inSorted(b.ServerNames, "api.b.test") {
		t.Fatalf("registration after a domain change: %+v", b)
	}
	if _, err := os.Stat(filepath.Join(x.sites(), "hasura.conf")); err != nil {
		t.Fatalf("the owner's hasura.conf must survive: %v", err)
	}
}

func TestSharedProxyPlugins(t *testing.T) {
	x := newSharedFx(t)
	x.plugin(t, "notify", 3709)
	x.mustBuild(t, x.owner, x.ownerEnv())
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
	conf := readText(t, filepath.Join(x.sites(), "bproj.notify-route.conf"))
	if !strings.Contains(conf, "bproj_notify:3709") || strings.Contains(conf, "http://notify:") {
		t.Errorf("B's notify upstream must be qualified:\n%s", conf)
	}
	if strings.Contains(readText(t, filepath.Join(x.sites(), "notify-route.conf")), "bproj_") {
		t.Error("the owner's notify conf must not be rewritten")
	}
	// The owner still builds although its bare notify host is now also B's service name.
	x.mustBuild(t, x.owner, x.ownerEnv())
}

func TestSharedProxyCollisions(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	guard := func() string { return treeDigest(t, filepath.Join(x.owner, "nginx"), filepath.Join(x.owner, "ssl")) }

	// E503: the same FQDN as the owner.
	before := guard()
	err := x.build(t, x.backend(), x.frontedEnv("bproj", "a.test"))
	wantCode(t, err, "E503", "api.a.test", "owna", "bproj")
	if guard() != before {
		t.Error("a refused build changed the owner's nginx/ssl tree")
	}

	// E504: a hand-written route to a bare owner service name.
	err = x.build(t, x.backend(), x.frontedEnv("bproj", "b.test", "INTERNAL_ROUTE_1_NAME=legacy", "INTERNAL_ROUTE_1_SUBDOMAIN=legacy", "INTERNAL_ROUTE_1_TARGET=http://hasura:8080"))
	wantCode(t, err, "E504", "hasura", "owna")
	if guard() != before {
		t.Error("an E504 build changed the owner's tree")
	}

	// E504 between two registered projects whose verbatim hosts collide.
	shared := []string{"INTERNAL_ROUTE_1_NAME=thing", "INTERNAL_ROUTE_1_SUBDOMAIN=thing", "INTERNAL_ROUTE_1_TARGET=http://sidecar:80"}
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test", shared...))
	before = guard()
	err = x.build(t, x.other(), x.frontedEnv("cproj", "c.test", shared...))
	wantCode(t, err, "E504", "sidecar", "bproj", "cproj")
	if guard() != before {
		t.Error("the colliding build changed the owner's tree")
	}

	// E503: a name another registered project serves.
	err = x.build(t, x.other(), x.frontedEnv("cproj", "b.test"))
	wantCode(t, err, "E503", "api.b.test", "bproj", "cproj")
}

func TestSharedProxyModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		for name, c := range map[string]struct {
			mode, front, provider string
			fail                  bool // fails in v1.4 too
		}{
			"bogus":          {"bogus", "", ProviderNginx, false},
			"shared+front":   {"shared", "stack", ProviderNginx, false},
			"shared+nginx":   {"shared", "", ProviderNginx, false},
			"shared+traefik": {"shared", "", ProviderTraefik, true},
		} {
			resetWarnOnce()
			cfg := &config.Config{}
			cfg.Nginx.Mode, cfg.Nginx.FrontedBy = c.mode, c.front
			st := &buildState{cfg: cfg}
			err := st.sharedModeCheck(c.provider)
			valid := cfg.NginxModeProblem() == ""
			v15 := os.Getenv("NSELF_V15") == "1"
			switch {
			case c.fail || (!valid && v15):
				wantCode(t, err, "E505")
			case err != nil:
				t.Errorf("%s: unexpected error %v", name, err)
			}
			if !valid && !v15 {
				if _, warned := fragmentWarned.Load(cfg.NginxModeProblem() + " (an error from v1.5)"); !warned {
					t.Errorf("%s: v1.4 must warn once", name)
				}
			}
		}
	})
}

// PROJECT_NAME=_owner is sanitised by the loader to "owner"; either way the
// owner's _owner.json is never overwritten (sharedproxy.ValidProject also
// refuses a leading underscore for any other path to the same name).
func TestSharedProxyReservedProjectName(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	before := readText(t, filepath.Join(x.shared(), sharedproxy.OwnerFile))
	_ = x.build(t, x.backend(), x.frontedEnv("_owner", "b.test"))
	if readText(t, filepath.Join(x.shared(), sharedproxy.OwnerFile)) != before {
		t.Fatal("_owner.json changed")
	}
}
