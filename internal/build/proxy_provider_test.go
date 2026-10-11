package build

import (
	"bytes"
	"context"
	"crypto/sha256"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx/routemodel"
)

func proxyFixture(t *testing.T) *planFixture {
	t.Helper()
	v15 := os.Getenv("NSELF_V15")
	f := newPlanFixture(t, "dev-minimal")
	t.Setenv("NSELF_V15", v15)
	f.plugins = filepath.Join(f.workdir, ".nself", "plugins")
	t.Setenv("NSELF_PLUGIN_DIR", f.plugins)
	// API_DOCS_* is not parsed (D-0455), so docs retain their enabled default.
	stubHostTools(t, f.root)
	writeFixtureFile(t, filepath.Join(f.root, "stub-bin", "nginx"), "#!/bin/sh\nexit 0\n", 0755)
	return f
}

func installProxy(t *testing.T, dir string) {
	t.Helper()
	writeFixtureFile(t, filepath.Join(dir, "traefik", "plugin.json"), `{"name":"traefik","version":"0.0.0-fixture"}`, 0644)
	writeFixtureFile(t, filepath.Join(dir, "traefik", "docker-compose.plugin.yml"), "services:\n  traefik:\n    image: traefik:v3.3\n", 0644)
}

type proxyFileSnapshot struct {
	Hash [32]byte
	Mode os.FileMode
}

func proxyHashes(t *testing.T, dir string) map[string]proxyFileSnapshot {
	t.Helper()
	out := map[string]proxyFileSnapshot{}
	err := filepath.Walk(dir, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if i.IsDir() {
			return nil
		}
		var b []byte
		if i.Mode()&os.ModeSymlink != 0 {
			var target string
			target, e = os.Readlink(p)
			b = []byte(target)
		} else {
			b, e = os.ReadFile(p)
		}
		if e != nil {
			return e
		}
		out[p] = proxyFileSnapshot{Hash: sha256.Sum256(b), Mode: i.Mode()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func freshProxyFixture(t *testing.T) *planFixture {
	t.Helper()
	f := proxyFixture(t)
	writeFixtureFile(t, filepath.Join(f.workdir, ".env"), "PROJECT_NAME=golden\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n", 0644)
	if err := os.Chmod(filepath.Join(f.workdir, ".env"), 0644); err != nil {
		t.Fatal(err)
	}
	return f
}

func declareProxy(t *testing.T, f *planFixture) *int {
	t.Helper()
	writeFixtureFile(t, filepath.Join(f.workdir, "nself.yaml"), "app: golden\nplugins:\n  free:\n    - traefik\n", 0644)
	t.Setenv("NSELF_AUTO_INSTALL_PLUGINS", "true")
	calls := new(int)
	previous := PluginInstall
	PluginInstall = func(_ context.Context, _ *config.Config, name, dir string) error {
		*calls++
		if name != "traefik" || dir != f.plugins {
			t.Fatalf("unexpected install: %s in %s", name, dir)
		}
		installProxy(t, dir)
		return nil
	}
	t.Cleanup(func() { PluginInstall = previous })
	return calls
}

func proxyRefusedBuild(t *testing.T, f *planFixture, names ...string) {
	t.Helper()
	for _, opts := range []BuildOptions{{Force: true}, {Check: true}, {Mode: ModePlan}} {
		before := proxyHashes(t, f.workdir)
		_, err := Build(f.workdir, opts)
		if err == nil {
			t.Fatal("build succeeded")
		}
		for _, s := range names {
			if !strings.Contains(err.Error(), s) {
				t.Errorf("missing %q: %v", s, err)
			}
		}
		if !reflect.DeepEqual(before, proxyHashes(t, f.workdir)) {
			t.Fatal("refused build changed project files")
		}
	}
}

func TestProxyProviderInstalled(t *testing.T) {
	t.Run("dry discovery records normalization without disk writes", func(t *testing.T) {
		f := freshProxyFixture(t)
		installProxy(t, f.plugins)
		writeFixtureFile(t, filepath.Join(f.plugins, "traefik", pluginComposeFilename), "version: '3.8'\nservices:\n  traefik:\n    image: traefik:v3.3\n", 0644)
		before := proxyHashes(t, f.workdir)
		fx := &planEffects{}
		files, err := discoverPluginComposeFilesFx(fx, newMemSink(f.workdir, ""), f.workdir, f.plugins)
		if err != nil || !reflect.DeepEqual(files, []string{filepath.Join(f.plugins, "traefik", pluginComposeFilename)}) {
			t.Fatalf("dry discovery: files=%v error=%v", files, err)
		}
		if len(fx.Recorded()) == 0 {
			t.Fatal("fixture must exercise planned normalization")
		}
		if !reflect.DeepEqual(before, proxyHashes(t, f.workdir)) {
			t.Fatal("dry discovery changed project files")
		}
	})
	t.Run("non-directory plugin paths mean nginx", func(t *testing.T) {
		f := proxyFixture(t)
		file := filepath.Join(f.workdir, "file")
		writeFixtureFile(t, file, "not a directory", 0644)
		for _, path := range []string{file, filepath.Join(file, "plugins"), f.plugins} {
			if path == f.plugins {
				writeFixtureFile(t, filepath.Join(path, "traefik"), "not a directory", 0644)
			}
			if p, err := ProxyProvider(nil, f.workdir, path); err != nil || p != ProviderNginx {
				t.Fatalf("%s: provider=%s error=%v", path, p, err)
			}
		}
	})
	f := proxyFixture(t)
	for _, tc := range []struct {
		name, path        string
		install, disabled bool
		want              string
	}{
		{"none", f.plugins, false, false, ProviderNginx},
		{"installed", f.plugins, true, false, ProviderTraefik},
		{"relative", ".nself/plugins", true, false, ProviderTraefik},
		{"disabled", f.plugins, true, true, ProviderNginx},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "relative" {
				t.Chdir(f.workdir)
			}
			if tc.install {
				installProxy(t, f.plugins)
			}
			if tc.disabled {
				writeFixtureFile(t, filepath.Join(f.plugins, "traefik", ".disabled"), "", 0644)
			}
			got, e := ProxyProvider(nil, f.workdir, tc.path)
			if e != nil || got != tc.want {
				t.Fatalf("%s: %v", got, e)
			}
		})
	}
	if err := os.Remove(filepath.Join(f.plugins, "traefik", ".disabled")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(f.plugins, "traefik")); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f.workdir)
	if err != nil {
		t.Fatal(err)
	}
	want, err := compose.NewGenerator(cfg).WithWorkDir(f.workdir).Generate()
	if err != nil {
		t.Fatal(err)
	}
	want = prependGeneratedHeader(TemplateSecrets(want, SecretEnvMap(cfg)))
	got, err := os.ReadFile(filepath.Join(f.workdir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatal("nginx build differs from generator bytes")
	}
	installProxy(t, f.plugins)
	t.Run("api docs enabled builds and warns once per build", func(t *testing.T) {
		var logs bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
		t.Cleanup(func() { slog.SetDefault(previous) })
		// Repeat to prove warning suppression does not leak into the next build.
		for range 2 {
			logs.Reset()
			if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
				t.Fatal(err)
			}
			if count := strings.Count(logs.String(), "docs."+cfg.BaseDomain+" is not served by traefik (D-0310)"); count != 1 {
				t.Fatalf("want one docs warning per build, got %d: %s", count, logs.String())
			}
			if _, err := os.Stat(filepath.Join(f.workdir, "nginx", "sites", "api-docs.conf")); err != nil {
				t.Fatal(err)
			}
		}
	})
	got, _ = os.ReadFile(filepath.Join(f.workdir, "docker-compose.yml"))
	if bytes.Contains(got, []byte("  nginx:\n")) {
		t.Fatal("nginx remains")
	}
	for _, p := range []string{"nginx/nginx.conf", ".nself/generated/routes.json"} {
		if _, e := os.Stat(filepath.Join(f.workdir, p)); e != nil {
			t.Fatal(e)
		}
	}
}

func TestProxyProviderSharedDirRefused(t *testing.T) {
	t.Run("installed compose manifest missing fragment refuses E480", func(t *testing.T) {
		f := freshProxyFixture(t)
		writeFixtureFile(t, filepath.Join(f.plugins, "traefik", "plugin.json"), `{"manifest_version":2,"name":"traefik","service":{"kind":"compose"}}`, 0644)
		proxyRefusedBuild(t, f, "E480", filepath.Join(f.plugins, "traefik", pluginComposeFilename))
	})
	t.Run("declared shared traefik refuses before install", func(t *testing.T) {
		f := freshProxyFixture(t)
		calls := declareProxy(t, f)
		shared := filepath.Join(f.home, ".nself", "plugins")
		t.Setenv("NSELF_PLUGIN_DIR", shared)
		proxyRefusedBuild(t, f, "E480", shared, "NSELF_PLUGIN_DIR")
		if *calls != 0 {
			t.Fatalf("refusal attempted %d installs", *calls)
		}
	})
	for _, name := range []string{"traefik", "traefik.prev"} {
		t.Run("compose-only "+name+" refuses without writes", func(t *testing.T) {
			f := freshProxyFixture(t)
			fragment := filepath.Join(f.plugins, name, pluginComposeFilename)
			writeFixtureFile(t, fragment, "version: '3.8'\nservices:\n  traefik:\n    image: traefik:v3.3\n", 0644)
			proxyRefusedBuild(t, f, "E480", "a traefik fragment would load while nginx serves 80/443", fragment)
		})
	}
	t.Run("relative directory uses cwd instead of workdir", func(t *testing.T) {
		f := proxyFixture(t)
		installProxy(t, f.plugins)
		t.Chdir(f.root)
		if provider, err := ProxyProvider(nil, f.workdir, ".nself/plugins"); err != nil || provider != ProviderNginx {
			t.Fatalf("absent cwd plugin: provider=%s error=%v", provider, err)
		}
		relative, err := filepath.Rel(f.root, f.plugins)
		if err != nil {
			t.Fatal(err)
		}
		if provider, err := ProxyProvider(nil, f.workdir, relative); err != nil || provider != ProviderTraefik {
			t.Fatalf("cwd-relative project plugin: provider=%s error=%v", provider, err)
		}
		outside := filepath.Join(f.root, ".nself", "plugins")
		installProxy(t, outside)
		if _, err := ProxyProvider(nil, f.workdir, ".nself/plugins"); err == nil || !strings.Contains(err.Error(), "E480") || !strings.Contains(err.Error(), outside) {
			t.Fatalf("cwd-relative shared plugin: %v", err)
		}
	})
	t.Run("symlinked traefik entry refused before writes", func(t *testing.T) {
		f := proxyFixture(t)
		if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
			t.Fatal(err)
		}
		realPlugins := filepath.Join(f.workdir, "real-plugins")
		installProxy(t, realPlugins)
		if err := os.MkdirAll(f.plugins, 0755); err != nil {
			t.Fatal(err)
		}
		entry := filepath.Join(f.plugins, "traefik")
		if err := os.Symlink(filepath.Join(realPlugins, "traefik"), entry); err != nil {
			t.Fatal(err)
		}
		proxyRefusedBuild(t, f, "E480", entry, "Symlinked plugin entries are not loaded", "real directory")
	})
	t.Run("tilde is a literal cwd path", func(t *testing.T) {
		f := proxyFixture(t)
		installProxy(t, filepath.Join(f.home, ".nself", "plugins"))
		t.Chdir(f.workdir)
		if provider, err := ProxyProvider(nil, f.workdir, "~/.nself/plugins"); err != nil || provider != ProviderNginx {
			t.Fatalf("home must not be expanded: provider=%s error=%v", provider, err)
		}
		installProxy(t, filepath.Join(f.workdir, "~", ".nself", "plugins"))
		if provider, err := ProxyProvider(nil, f.workdir, "~/.nself/plugins"); err != nil || provider != ProviderTraefik {
			t.Fatalf("literal cwd plugin: provider=%s error=%v", provider, err)
		}
	})
	f := proxyFixture(t)
	if _, e := Build(f.workdir, BuildOptions{Force: true}); e != nil {
		t.Fatal(e)
	}
	shared := filepath.Join(f.home, ".nself", "plugins")
	installProxy(t, shared)
	t.Setenv("NSELF_PLUGIN_DIR", "")
	proxyRefusedBuild(t, f, "E480", "NSELF_PLUGIN_DIR", shared)
	for _, path := range []string{shared, f.workdir} {
		if path == f.workdir {
			installProxy(t, path)
		}
		if _, e := ProxyProvider(nil, f.workdir, path); e == nil || !strings.Contains(e.Error(), "E480") {
			t.Fatalf("%s: %v", path, e)
		}
	}
	link := filepath.Join(f.workdir, "link")
	if e := os.Symlink(shared, link); e != nil {
		t.Fatal(e)
	}
	if _, e := ProxyProvider(nil, f.workdir, link); e == nil || !strings.Contains(e.Error(), "E480") {
		t.Fatalf("symlink: %v", e)
	}
	installProxy(t, f.plugins)
	if p, e := ProxyProvider(nil, f.workdir, f.plugins); e != nil || p != ProviderTraefik {
		t.Fatalf("local: %s %v", p, e)
	}
}

func TestProxyRefusalFronted(t *testing.T) {
	t.Run("declared traefik refuses before install or writes", func(t *testing.T) {
		f := freshProxyFixture(t)
		calls := declareProxy(t, f)
		t.Setenv("NGINX_FRONTED_BY", "fronting")
		proxyRefusedBuild(t, f, "E482")
		if *calls != 0 {
			t.Fatalf("refusal attempted %d installs", *calls)
		}
	})
	for _, tc := range []struct{ name, compose, manifest string }{
		{"fronting compose lacks nginx", "services:\n  traefik:\n    image: traefik:v3.3\n", ""},
		{"fronting manifest loads traefik", "services:\n  nginx:\n    image: nginx:alpine\n", "traefik"},
		{"fronting manifest loads traefik.prev", "services:\n  nginx:\n    image: nginx:alpine\n", "traefik.prev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := freshProxyFixture(t)
			front := filepath.Join(f.root, "fronting")
			backend := filepath.Join(front, "backend")
			if err := os.MkdirAll(front, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(f.workdir, backend); err != nil {
				t.Fatal(err)
			}
			f.workdir = backend
			f.plugins = filepath.Join(backend, ".nself", "plugins")
			t.Setenv("NSELF_PLUGIN_DIR", f.plugins)
			t.Setenv("NGINX_FRONTED_BY", "fronting")
			writeFixtureFile(t, filepath.Join(front, "nginx", "sites", "front.conf"), "server {}\n", 0644)
			writeFixtureFile(t, filepath.Join(front, "docker-compose.yml"), ComposeGeneratedHeader+tc.compose, 0644)
			if tc.manifest != "" {
				writeFixtureFile(t, filepath.Join(front, ".nself", "compose-files.txt"), "docker-compose.yml\n"+filepath.Join(front, ".nself", "plugins", tc.manifest, pluginComposeFilename)+"\n", 0644)
			}
			before := proxyHashes(t, front)
			proxyRefusedBuild(t, f, "E482", front)
			if !reflect.DeepEqual(before, proxyHashes(t, front)) {
				t.Fatal("refused backend build changed fronting project")
			}
		})
	}
	f := proxyFixture(t)
	if _, e := Build(f.workdir, BuildOptions{Force: true}); e != nil {
		t.Fatal(e)
	}
	installProxy(t, f.plugins)
	t.Setenv("NGINX_FRONTED_BY", "fronting")
	proxyRefusedBuild(t, f, "E482")
}

func TestProxyRefusalUnmodelled(t *testing.T) {
	t.Run("fresh project refuses before secret persistence or chmod", func(t *testing.T) {
		f := freshProxyFixture(t)
		installProxy(t, f.plugins)
		writeFixtureFile(t, filepath.Join(f.plugins, "idme", "nginx", "idme.conf"), idmeSnippet, 0644)
		if _, err := os.Stat(filepath.Join(f.workdir, ".env.secrets")); !os.IsNotExist(err) {
			t.Fatalf("fresh project already has secrets file: %v", err)
		}
		proxyRefusedBuild(t, f, "E481", "weird_thing")
	})
	t.Run("declared traefik refuses unmodelled conf before install", func(t *testing.T) {
		f := freshProxyFixture(t)
		calls := declareProxy(t, f)
		writeFixtureFile(t, filepath.Join(f.workdir, "nginx", "conf.d", "unsupported.conf"), "server { listen 80; server_name other.example.test; weird_thing on; location / { proxy_pass http://hasura:8080; } }", 0644)
		proxyRefusedBuild(t, f, "E481", "weird_thing")
		if *calls != 0 {
			t.Fatalf("refusal attempted %d installs", *calls)
		}
	})
	t.Run("disabled auto-install leaves declared traefik on nginx", func(t *testing.T) {
		f := proxyFixture(t)
		calls := declareProxy(t, f)
		t.Setenv("NSELF_AUTO_INSTALL_PLUGINS", "false")
		writeFixtureFile(t, filepath.Join(f.workdir, "nginx", "conf.d", "unsupported.conf"), "server { listen 80; server_name other.example.test; weird_thing on; location / { proxy_pass http://hasura:8080; } }", 0644)
		if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(f.workdir, "docker-compose.yml"))
		if err != nil || !bytes.Contains(b, []byte("  nginx:\n")) || *calls != 0 {
			t.Fatalf("nginx disabled-install build: calls=%d error=%v compose=%s", *calls, err, b)
		}
	})
	t.Run("step eight rechecks newly installed provider routes", func(t *testing.T) {
		f := proxyFixture(t)
		cfg, err := config.Load(f.workdir)
		if err != nil {
			t.Fatal(err)
		}
		model, err := routemodel.Build(cfg, f.workdir, false)
		if err != nil {
			t.Fatal(err)
		}
		st := &buildState{cfg: cfg, workdir: f.workdir, pluginDir: f.plugins, routes: model}
		installProxy(t, f.plugins)
		writeFixtureFile(t, filepath.Join(f.workdir, "nginx", "conf.d", "late.conf"), "server { listen 80; server_name late.example.test; weird_thing on; location / { proxy_pass http://hasura:8080; } }", 0644)
		before := proxyHashes(t, f.workdir)
		if err := st.applyProxyProvider(compose.NewGenerator(cfg)); err == nil || !strings.Contains(err.Error(), "E481") || !strings.Contains(err.Error(), "weird_thing") {
			t.Fatalf("Step 8 failed to recheck routes: %v", err)
		}
		if !reflect.DeepEqual(before, proxyHashes(t, f.workdir)) {
			t.Fatal("Step 8 dry discovery changed project files")
		}
	})
	f := proxyFixture(t)
	if _, e := Build(f.workdir, BuildOptions{Force: true}); e != nil {
		t.Fatal(e)
	}
	installProxy(t, f.plugins)
	writeFixtureFile(t, filepath.Join(f.plugins, "idme", "nginx", "idme.conf"), idmeSnippet, 0644)
	writeFixtureFile(t, filepath.Join(f.workdir, "nginx", "conf.d", "shadow.conf"), "server { listen 80; server_name api.example.test; location / { proxy_pass http://hasura:8080; } }", 0644)
	proxyRefusedBuild(t, f, "E481", "plugin:idme/idme.conf#2", "weird_thing", "core:hasura", "shadow.conf", "global:")
	// Exhaustive aggregation and deterministic ordering also cover upstream loss.
	cfg := &config.Config{}
	cfg.ApiDocs.Path = "/docs"
	s := "shadow.conf"
	m := &routemodel.Model{Routes: []routemodel.Route{{ID: "z", Source: "core", RawUpstream: "http://up:80/path?q=1", Unmodelled: []string{"z directive", "a directive"}, ShadowedBy: &s}}, UnmodelledGlobal: []string{"map x"}}
	e := checkProviderRoutes(m, ProviderTraefik, cfg, t.TempDir())
	if e == nil {
		t.Fatal("accepted lossy routes")
	}
	for _, item := range []string{"upstream http://up:80/path?q=1", "global: map x", "unmodelled z directive", "unmodelled a directive", "shadowed by shadow.conf"} {
		if !strings.Contains(e.Error(), item) {
			t.Errorf("missing %s: %v", item, e)
		}
	}
	if strings.Index(e.Error(), "unmodelled a") > strings.Index(e.Error(), "unmodelled z") {
		t.Fatal("unsorted refusals")
	}
	for _, tc := range []struct {
		raw     string
		refused bool
	}{
		{"http://up:80/path", true}, {"http://up:80?q=1", true},
		{"http://up:80/", false}, {"http://up:80", false},
		{"up:80/path", true}, {"up:80?q=1", true}, {"up:80/", false}, {"up:80", false},
		{"up:80/%zz", true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			model := &routemodel.Model{Routes: []routemodel.Route{{ID: "cs:1", Source: "custom_service", RawUpstream: tc.raw}}}
			err := checkProviderRoutes(model, ProviderTraefik, cfg, t.TempDir())
			if (err != nil) != tc.refused {
				t.Fatalf("refused=%v error=%v", tc.refused, err)
			}
			if err != nil && !strings.Contains(err.Error(), "route cs:1: upstream "+tc.raw) {
				t.Fatal(err)
			}
			if tc.raw == "up:80/%zz" && !strings.Contains(err.Error(), "cannot be parsed") {
				t.Fatal(err)
			}
		})
	}
}

func TestProxyRefusalForeignConf(t *testing.T) {
	t.Run("owned model conf", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.Config{}
		cfg.ApiDocs.Path = "/docs"
		cfg.PluginSystem.Dir = filepath.Join(dir, "plugins")
		writeFixtureFile(t, filepath.Join(dir, "nginx", "sites", "hasura.conf"), nginxGeneratedMarker+"\nserver {}", 0644)
		m := &routemodel.Model{Routes: []routemodel.Route{{ID: "core:hasura", Source: "core", File: "nginx/sites/hasura.conf"}}}
		if err := checkProviderRoutes(m, ProviderTraefik, cfg, dir); err != nil {
			t.Fatal(err)
		}
	})
	for _, tc := range []struct{ name, body, want string }{
		{"foreign.conf", "# foreign\nserver {}", "not generated by nself"},
		{"other.conf", nginxGeneratedMarker + "\nserver {}", "generated by another build"},
		{"hasura.conf", nginxGeneratedMarker + "\nserver {}", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := proxyFixture(t)
			if _, e := Build(f.workdir, BuildOptions{Force: true}); e != nil {
				t.Fatal(e)
			}
			installProxy(t, f.plugins)
			writeFixtureFile(t, filepath.Join(f.workdir, "nginx", "sites", tc.name), tc.body, 0644)
			if tc.want != "" {
				proxyRefusedBuild(t, f, "E481", tc.name, tc.want)
			} else if _, e := Build(f.workdir, BuildOptions{Force: true}); e != nil {
				t.Fatal(e)
			}
		})
	}
	t.Run("enabled api docs conf is owned", func(t *testing.T) {
		f := proxyFixture(t)
		if _, e := Build(f.workdir, BuildOptions{Force: true}); e != nil {
			t.Fatal(e)
		}
		installProxy(t, f.plugins)
		cfg, err := config.Load(f.workdir)
		if err != nil {
			t.Fatal(err)
		}
		cfg.ApiDocs.Enabled = true
		cfg.PluginSystem.Dir = f.plugins
		// The generated docs site remains this build's own file under traefik.
		docsOnly := t.TempDir()
		writeFixtureFile(t, filepath.Join(docsOnly, "nginx", "sites", "api-docs.conf"), nginxGeneratedMarker+"\nserver {}", 0644)
		if err := checkProviderRoutes(&routemodel.Model{}, ProviderTraefik, cfg, docsOnly); err != nil {
			t.Fatalf("docs conf reported as foreign: %v", err)
		}
		if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
			t.Fatal(err)
		}
	})
}
