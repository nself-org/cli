package build

// plugin_nginx_routes_build_test.go — plugin snippets and hand-managed conf.d
// files in the routes.json a build writes (P7-DEPL-22), through Build.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/cli/internal/nginx/routemodel"
)

const idmeSnippet = `upstream idme_backend { server 127.0.0.1:3010; keepalive 32; }
server { listen 80; server_name idme.${BASE_DOMAIN}; return 301 https://$host$request_uri; }
server {
    listen 443 ssl http2;
    server_name idme.${BASE_DOMAIN};
    ssl_certificate /etc/nginx/ssl/${BASE_DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/${BASE_DOMAIN}/privkey.pem;
    location / { proxy_pass http://idme_backend; proxy_read_timeout 60s; weird_thing on; }
}
`

func routesOf(t *testing.T, workdir string) (map[string]routemodel.Route, routemodel.Model, []byte) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(workdir, ".nself", "generated", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "schemas", "proxy-routes.v1.schema.json"))
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
	if err := json.Unmarshal(b, &instance); err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(instance); err != nil {
		t.Fatalf("routes.json violates schema: %v", err)
	}
	var m routemodel.Model
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	byID := map[string]routemodel.Route{}
	for _, r := range m.Routes {
		byID[r.ID] = r
	}
	return byID, m, b
}

// nginxTree reads every file under workdir/nginx keyed by relative path.
func nginxTree(t *testing.T, workdir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	root := filepath.Join(workdir, "nginx")
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			out[rel] = string(b)
		}
		return nil
	})
	return out
}

func TestPluginRoutesInRoutesJSON(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	before := nginxTree(t, f.workdir)
	_, _, plain := routesOf(t, f.workdir)

	writeFixtureFile(t, filepath.Join(f.plugins, "idme", "nginx", "idme.conf"), idmeSnippet, 0o644)
	if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	after := nginxTree(t, f.workdir)
	byID, m, withPlugin := routesOf(t, f.workdir)

	// nginx output: the only change is the copied snippet, byte for byte.
	want := nginxGeneratedMarker + "\n" + strings.ReplaceAll(idmeSnippet, "${BASE_DOMAIN}", "example.test")
	if after["sites/idme-idme.conf"] != want {
		t.Errorf("copied snippet changed:\n%q", after["sites/idme-idme.conf"])
	}
	delete(after, "sites/idme-idme.conf")
	if !reflect.DeepEqual(before, after) {
		t.Error("the rest of the nginx tree changed when a plugin snippet was added")
	}
	r1, r2 := byID["plugin:idme/idme.conf#1"], byID["plugin:idme/idme.conf#2"]
	if r1.Source != "plugin" || *r1.Owner != "idme" || !r1.HTTPToHTTPSRedirect || !r2.Listen.HTTPS || r2.TLS == nil || r2.TLS.SSLDir != "example.test" ||
		*r2.Locations[0].Upstream != (routemodel.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 3010}) || *r2.Locations[0].Timeouts.ReadS != 60 ||
		!reflect.DeepEqual(r2.Unmodelled, []string{"weird_thing on;"}) || len(m.UnmodelledGlobal) != 0 {
		t.Errorf("plugin routes wrong: %+v / %+v", r1, r2)
	}
	// the generated routes are untouched and plugin routes are the only addition
	var pm routemodel.Model
	if err := json.Unmarshal(plain, &pm); err != nil {
		t.Fatal(err)
	}
	var generated []routemodel.Route
	for _, r := range m.Routes {
		if r.Source != "plugin" {
			generated = append(generated, r)
		}
	}
	if !reflect.DeepEqual(generated, pm.Routes) || len(m.Routes) != len(pm.Routes)+2 {
		t.Error("generated routes changed")
	}
	// a second build is byte-stable, and removing the plugin removes its routes
	if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, again := routesOf(t, f.workdir); !bytes.Equal(again, withPlugin) {
		t.Error("routes.json is not stable across builds")
	}
	if err := os.RemoveAll(filepath.Join(f.plugins, "idme")); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, gone := routesOf(t, f.workdir); !bytes.Equal(gone, plain) {
		t.Error("routes.json kept a removed plugin's routes")
	}
}

// TestPluginRoutesConfirmedRender: routes.json is written once with the planned
// bytes, so a build held to its confirmed plan (reconcile.Apply) still passes.
func TestPluginRoutesConfirmedRender(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	writeFixtureFile(t, filepath.Join(f.plugins, "idme", "nginx", "idme.conf"), idmeSnippet, 0o644)
	writeFixtureFile(t, filepath.Join(f.workdir, "nginx", "conf.d", "custom.conf"), "server { listen 80; server_name h.example.test; }\n", 0o644)
	plan, err := Build(f.workdir, BuildOptions{Mode: ModePlan, Rand: newSeedStream("seed")})
	if err != nil || plan.Planned == nil {
		t.Fatalf("plan: %v", err)
	}
	planned := plan.Planned.Files[".nself/generated/routes.json"]
	if !bytes.Contains(planned.Data, []byte("plugin:idme/idme.conf#2")) || !bytes.Contains(planned.Data, []byte("hand:custom.conf#1")) {
		t.Fatal("the plan does not carry the plugin and hand-managed routes")
	}
	if _, err := Build(f.workdir, BuildOptions{Expect: plan.Planned, Rand: newSeedStream("seed")}); err != nil {
		t.Fatalf("apply held to the plan: %v", err)
	}
	if _, _, got := routesOf(t, f.workdir); !bytes.Equal(got, planned.Data) {
		t.Error("routes.json differs from the plan")
	}
}

func TestHandManagedRoutesJSON(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	conf := filepath.Join(f.workdir, "nginx")
	writeFixtureFile(t, filepath.Join(conf, "conf.d", "custom.conf"), `map $http_upgrade $up { default upgrade; }
server { listen 80; server_name a.example.test; location / { proxy_pass http://a:80; } }
server { listen 80; server_name b.example.test; location /x { proxy_pass http://b:81; } }
`, 0o644)
	writeFixtureFile(t, filepath.Join(conf, "conf.d-dev", "extra.conf"), "server { listen 80; server_name c.example.test; }\n", 0o644)
	writeFixtureFile(t, filepath.Join(conf, "conf.d", "default.conf"), "server { listen 80; server_name _; }\n", 0o644)
	writeFixtureFile(t, filepath.Join(conf, "conf.d", "notes.txt"), "server { listen 80; server_name no.example.test; }\n", 0o644)
	if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	byID, m, _ := routesOf(t, f.workdir)
	var hand []string
	for id := range byID {
		if strings.HasPrefix(id, "hand:") {
			hand = append(hand, id)
		}
	}
	sort.Strings(hand)
	if want := []string{"hand:conf.d-dev/extra.conf#1", "hand:custom.conf#1", "hand:custom.conf#2"}; !reflect.DeepEqual(hand, want) {
		t.Fatalf("hand routes = %v, want %v", hand, want)
	}
	r := byID["hand:custom.conf#2"]
	if r.Source != "hand_managed" || r.Owner != nil || r.File != "nginx/conf.d/custom.conf" || r.ServerNames[0] != "b.example.test" ||
		r.Locations[0].Path != "/x" || r.Locations[0].Upstream.Port != 81 {
		t.Errorf("hand route wrong: %+v", r)
	}
	if byID["hand:conf.d-dev/extra.conf#1"].File != "nginx/conf.d-dev/extra.conf" {
		t.Error("conf.d-dev file path wrong")
	}
	if !reflect.DeepEqual(m.UnmodelledGlobal, []string{"map $http_upgrade $up { default upgrade; }"}) {
		t.Errorf("unmodelled_global = %q", m.UnmodelledGlobal)
	}
}
