package build

// plugin_nginx_routes_test.go — the parseServerBlocks adapter over the shared
// tokenizer, and the plugin and hand-managed routes withSnippetRoutes adds to
// a model (P7-DEPL-22).
//
// Test-only addition outside the Ticket's write_scope list (the Ticket names
// no build test file); it adds tests and changes no existing one.

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx/routemodel"
)

const unitSnippet = `upstream idme_backend { server 127.0.0.1:3010; keepalive 32; }
server { listen 80; server_name idme.${BASE_DOMAIN}; return 301 https://$host$request_uri; }
server {
    listen 443 ssl http2;
    server_name idme.${BASE_DOMAIN};
    ssl_certificate /etc/nginx/ssl/${BASE_DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/${BASE_DOMAIN}/privkey.pem;
    location / { proxy_pass http://idme_backend; proxy_read_timeout 60s; weird_thing on; }
}
`

// snippetModel returns the generated model of a project at workdir and the
// state withSnippetRoutes needs.
func snippetModel(t *testing.T, workdir, pluginDir string) (*buildState, *routemodel.Model) {
	t.Helper()
	cfg := &config.Config{ProjectName: "unit", BaseDomain: "example.test", Env: "dev"}
	cfg.PluginSystem.Dir = pluginDir
	m, err := routemodel.Build(cfg, workdir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &buildState{cfg: cfg, workdir: workdir}, m
}

func TestPluginSnippetRoutes(t *testing.T) {
	workdir, plugins := t.TempDir(), t.TempDir()
	writeFixtureFile(t, filepath.Join(plugins, "idme", "nginx", "idme.conf"), unitSnippet, 0o644)
	st, m := snippetModel(t, workdir, plugins)
	out := st.withSnippetRoutes(m)
	byID := map[string]routemodel.Route{}
	for _, r := range out.Routes {
		byID[r.ID] = r
	}
	r1, r2 := byID["plugin:idme/idme.conf#1"], byID["plugin:idme/idme.conf#2"]
	if r1.Source != "plugin" || *r1.Owner != "idme" || !r1.HTTPToHTTPSRedirect || !r2.Listen.HTTPS || r2.TLS == nil || r2.TLS.SSLDir != "example.test" ||
		*r2.Locations[0].Upstream != (routemodel.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 3010}) || *r2.Locations[0].Timeouts.ReadS != 60 ||
		!reflect.DeepEqual(r2.Unmodelled, []string{"weird_thing on;"}) || len(out.UnmodelledGlobal) != 0 {
		t.Errorf("plugin routes wrong: %+v / %+v", r1, r2)
	}
	if len(m.Routes)+2 != len(out.Routes) || len(m.UnmodelledGlobal) != 0 {
		t.Error("withSnippetRoutes changed the model it was given")
	}
	// the injected file is the marker plus the rendered snippet, byte for byte
	if n, err := InjectPluginNginxRoutes(workdir, plugins, st.cfg); err != nil || n != 1 {
		t.Fatalf("inject: %d %v", n, err)
	}
	got, err := os.ReadFile(filepath.Join(workdir, "nginx", "sites", "idme-idme.conf"))
	want := nginxGeneratedMarker + "\n" + strings.ReplaceAll(unitSnippet, "${BASE_DOMAIN}", "example.test")
	if err != nil || string(got) != want {
		t.Errorf("injected snippet changed: %v\n%q", err, got)
	}
	// a snippet that does not parse is recorded, not dropped
	writeFixtureFile(t, filepath.Join(plugins, "broken", "nginx", "b.conf"), "server { listen 80;", 0o644)
	if out := st.withSnippetRoutes(m); len(out.UnmodelledGlobal) != 1 || !strings.HasPrefix(out.UnmodelledGlobal[0], "unparsed plugin broken/b.conf") {
		t.Errorf("unparsed snippet not recorded: %q", out.UnmodelledGlobal)
	}
}

func TestHandManagedFiles(t *testing.T) {
	workdir := t.TempDir()
	conf := filepath.Join(workdir, "nginx")
	writeFixtureFile(t, filepath.Join(conf, "conf.d", "custom.conf"), `map $http_upgrade $up { default upgrade; }
server { listen 80; server_name a.example.test; location / { proxy_pass http://a:80; } }
server { listen 80; server_name b.example.test; location /x { proxy_pass http://b:81; } }
`, 0o644)
	writeFixtureFile(t, filepath.Join(conf, "conf.d-dev", "extra.conf"), "server { listen 80; server_name c.example.test; }\n", 0o644)
	writeFixtureFile(t, filepath.Join(conf, "conf.d-prod", "other.conf"), "server { listen 80; server_name p.example.test; }\n", 0o644)
	writeFixtureFile(t, filepath.Join(conf, "conf.d", "default.conf"), "server { listen 80; server_name _; }\n", 0o644)
	writeFixtureFile(t, filepath.Join(conf, "conf.d", "notes.txt"), "server { listen 80; server_name no.example.test; }\n", 0o644)
	writeFixtureFile(t, filepath.Join(conf, "conf.d", "gen.conf"), nginxGeneratedMarker+"\nserver { listen 80; server_name g.example.test; }\n", 0o644)
	st, m := snippetModel(t, workdir, t.TempDir())
	out := st.withSnippetRoutes(m)
	byID := map[string]routemodel.Route{}
	var hand []string
	for _, r := range out.Routes {
		byID[r.ID] = r
		if r.Source == "hand_managed" {
			hand = append(hand, r.ID)
		}
	}
	sort.Strings(hand)
	if want := []string{"hand:conf.d-dev/extra.conf#1", "hand:custom.conf#1", "hand:custom.conf#2"}; !reflect.DeepEqual(hand, want) {
		t.Fatalf("hand routes = %v, want %v", hand, want)
	}
	r := byID["hand:custom.conf#2"]
	if r.Owner != nil || r.File != "nginx/conf.d/custom.conf" || r.ServerNames[0] != "b.example.test" || r.Locations[0].Path != "/x" || r.Locations[0].Upstream.Port != 81 {
		t.Errorf("hand route wrong: %+v", r)
	}
	if byID["hand:conf.d-dev/extra.conf#1"].File != "nginx/conf.d-dev/extra.conf" {
		t.Error("conf.d-dev file path wrong")
	}
	if !reflect.DeepEqual(out.UnmodelledGlobal, []string{"map $http_upgrade $up { default upgrade; }"}) {
		t.Errorf("unmodelled_global = %q", out.UnmodelledGlobal)
	}
}

func TestParseServerBlocksAdapter(t *testing.T) {
	type blk struct {
		names []string
		ports []string
	}
	cases := map[string][]blk{
		"compact":        {{[]string{"a.test"}, []string{"443", "443"}}},
		"quote":          {{[]string{"q.test"}, nil}},
		"nested":         {{[]string{"n.test"}, []string{"80"}}, {[]string{"inner.test"}, []string{"81"}}},
		"no semicolon":   {{[]string{"s.test"}, nil}},
		"comment braces": {{[]string{"c.test"}, nil}},
	}
	in := map[string]string{
		"compact":        `server{listen 443 ssl;listen [::]:443 ssl;server_name a.test;location /{proxy_pass http://u:1;}}`,
		"quote":          `server { return 200 "a;b{c}"; server_name q.test; }`,
		"nested":         `http { server { listen 80; server_name n.test; server { listen 81; server_name inner.test; } } upstream u { server x:1; } }`,
		"no semicolon":   `server { server_name s.test }`,
		"comment braces": "server { # } listen 99;\n server_name c.test; }",
	}
	for name, text := range in {
		var got []blk
		for _, b := range parseServerBlocks(text) {
			got = append(got, blk{b.ServerNames, b.Ports})
		}
		if !reflect.DeepEqual(got, cases[name]) {
			t.Errorf("%s: parseServerBlocks = %+v, want %+v", name, got, cases[name])
		}
	}
	// the build conflict check still sees plugin-shaped confs (claimsOf over the adapter)
	if c := claimsOf(`server{listen 80;server_name a.test b.test;}`); !c["80|a.test"] || !c["80|b.test"] || len(c) != 2 {
		t.Errorf("claimsOf = %v", c)
	}
}
