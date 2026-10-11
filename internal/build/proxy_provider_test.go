package build

import (
	"bytes"
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
	// API_DOCS_* is not parsed (D-0435), so docs retain their enabled default.
	stubHostTools(t, f.root)
	writeFixtureFile(t, filepath.Join(f.root, "stub-bin", "nginx"), "#!/bin/sh\nexit 0\n", 0755)
	return f
}

func installProxy(t *testing.T, dir string) {
	t.Helper()
	writeFixtureFile(t, filepath.Join(dir, "traefik", "plugin.json"), `{"name":"traefik","version":"0.0.0-fixture"}`, 0644)
}

func proxyHashes(t *testing.T, dir string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	err := filepath.Walk(dir, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if i.IsDir() {
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		out[p] = sha256.Sum256(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
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
	f := proxyFixture(t)
	if _, e := Build(f.workdir, BuildOptions{Force: true}); e != nil {
		t.Fatal(e)
	}
	shared := filepath.Join(f.home, ".nself", "plugins")
	installProxy(t, shared)
	t.Setenv("NSELF_PLUGIN_DIR", "")
	proxyRefusedBuild(t, f, "E480", "NSELF_PLUGIN_DIR", shared)
	for _, path := range []string{shared, "~/.nself/plugins", f.workdir} {
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
	f := proxyFixture(t)
	if _, e := Build(f.workdir, BuildOptions{Force: true}); e != nil {
		t.Fatal(e)
	}
	installProxy(t, f.plugins)
	t.Setenv("NGINX_FRONTED_BY", "fronting")
	proxyRefusedBuild(t, f, "E482")
}

func TestProxyRefusalUnmodelled(t *testing.T) {
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
