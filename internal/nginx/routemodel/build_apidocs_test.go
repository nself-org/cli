package routemodel

// Purpose: tests for the api-docs route, upstream path/query parsing and the
//          omitempty JSON of the new Upstream/Location fields (D-0310).
// Inputs:  in-memory configs; no filesystem state beyond temp dirs.
// Outputs: none (t.Error on assertion failure).

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func strp(s string) *string { return &s }

// TestAPIDocsRoute covers the on/off rule, the docs path and the server name.
func TestAPIDocsRoute(t *testing.T) {
	cases := []struct {
		name     string
		cfg      config.Config
		wantNil  bool
		wantName string
		wantPath string
	}{
		{"zero section is on", config.Config{BaseDomain: "example.test"}, false, "docs.example.test", "/docs"},
		{"loader output for enabled false", config.Config{BaseDomain: "example.test",
			ApiDocs: config.ApiDocsConfig{Enabled: false, Path: "/docs"}}, true, "", ""},
		{"custom path", config.Config{BaseDomain: "example.test",
			ApiDocs: config.ApiDocsConfig{Enabled: true, Path: "/reference"}}, false, "docs.example.test", "/reference"},
		{"empty base domain", config.Config{}, false, "docs.localhost", "/docs"},
		{"app name is not a suffix", config.Config{BaseDomain: "example.test", AppName: "shop"}, false, "docs.example.test", "/docs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			r, err := APIDocs(&cfg, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantNil {
				if r != nil {
					t.Fatalf("route = %+v, want nil", r)
				}
				return
			}
			if r == nil {
				t.Fatal("route is nil")
			}
			if r.ID != "core:api-docs" || r.File != "nginx/sites/api-docs.conf" || r.Source != "core" ||
				!reflect.DeepEqual(r.ServerNames, []string{tc.wantName}) {
				t.Errorf("bad identity: %+v", r)
			}
			if r.Listen.HTTP || !r.Listen.HTTPS || r.HTTPToHTTPSRedirect || r.TLS == nil || r.ShadowedBy != nil || r.RateLimit != nil {
				t.Errorf("bad listen/tls: %+v", r)
			}
			if want := strings.ReplaceAll(strings.TrimPrefix(tc.wantName, "docs."), ".", "-"); r.TLS.SSLDir != want {
				t.Errorf("SSLDir = %q, want %q", r.TLS.SSLDir, want)
			}
			if len(r.Locations) != 2 || r.Locations[0].Path != "/api-docs" || r.Locations[1].Path != tc.wantPath ||
				*r.Locations[0].StaticFile != "/opt/nself/.dist/openapi.json" ||
				*r.Locations[1].StaticFile != "/opt/nself/.dist/scalar.html" ||
				r.Locations[0].HeadersAdd["Access-Control-Allow-Origin"] != "*" ||
				r.Locations[1].HeadersAdd["Content-Type"] != "text/html" {
				t.Errorf("bad locations: %+v", r.Locations)
			}
		})
	}
}

// TestAPIDocsRejectsBadPaths checks the nginx-unsafe and colliding paths, and
// that Build refuses them before anything is written.
func TestAPIDocsRejectsBadPaths(t *testing.T) {
	for _, p := range []string{"/x;", "/x {", "docs", "/api-docs", "/api-docsx", "/a b", "/x\n"} {
		cfg := &config.Config{BaseDomain: "example.test", ApiDocs: config.ApiDocsConfig{Enabled: true, Path: p}}
		if _, err := APIDocs(cfg, t.TempDir()); err == nil || !strings.Contains(err.Error(), "API_DOCS_PATH") {
			t.Errorf("APIDocs accepted path %q: %v", p, err)
		}
		if _, err := Build(cfg, t.TempDir(), false); err == nil || !strings.Contains(err.Error(), "API_DOCS_PATH") {
			t.Errorf("Build accepted path %q: %v", p, err)
		}
	}
}

// TestAPIDocsNotInBuildRoutes keeps the route out of Build's rendered routes.
func TestAPIDocsNotInBuildRoutes(t *testing.T) {
	m, err := Build(&config.Config{BaseDomain: "example.test"}, t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range m.Routes {
		if r.ID == "core:api-docs" {
			t.Fatal("api-docs route must be appended at routes.json time, not by Build")
		}
	}
}

// TestParseUpstreamPathQuery checks that a target's path and query survive.
func TestParseUpstreamPathQuery(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want Upstream
	}{
		{"hasura:8080", Upstream{Scheme: "http", Host: "hasura", Port: 8080}},
		{"http://h:1/x?a=b", Upstream{Scheme: "http", Host: "h", Port: 1, Path: "/x", Query: strp("a=b")}},
		{"https://h/", Upstream{Scheme: "https", Host: "h", Port: 443, Path: "/"}},
		{"http://h/p?", Upstream{Scheme: "http", Host: "h", Port: 80, Path: "/p", Query: strp("")}},
		{"h", Upstream{Scheme: "http", Host: "h", Port: 80}},
		{"http://h:2/a%20b", Upstream{Scheme: "http", Host: "h", Port: 2, Path: "/a%20b"}},
	} {
		if got := parseUpstream(tc.raw); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseUpstream(%q) = %+v, want %+v", tc.raw, got, tc.want)
		}
	}
}

// TestMarshalOmitsNewFields proves a path-less upstream emits no path/query and
// the api-docs locations emit static_file and headers_add.
func TestMarshalOmitsNewFields(t *testing.T) {
	plain := &Model{Routes: []Route{{ID: "a", Locations: []Location{{Path: "/", Match: "prefix",
		Upstream: &Upstream{Scheme: "http", Host: "h", Port: 80}}}}}}
	b, err := Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"path": "/"`, `"query"`, `"static_file"`, `"headers_add"`} {
		if key == `"path": "/"` {
			continue // location path is always present
		}
		if strings.Contains(string(b), key) {
			t.Errorf("plain model emitted %s:\n%s", key, b)
		}
	}
	if strings.Count(string(b), `"path"`) != 1 {
		t.Errorf("upstream emitted a path key:\n%s", b)
	}
	r, err := APIDocs(&config.Config{BaseDomain: "example.test"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err = Marshal(&Model{Routes: []Route{*r}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"static_file": "/opt/nself/.dist/openapi.json"`, `"headers_add": {`, `"Access-Control-Allow-Origin": "*"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("api-docs JSON missing %s:\n%s", want, b)
		}
	}
	withQuery := &Model{Routes: []Route{{ID: "q", Locations: []Location{{Path: "/", Match: "prefix",
		Upstream: &Upstream{Scheme: "http", Host: "h", Port: 80, Path: "/p", Query: strp("")}}}}}}
	b, _ = Marshal(withQuery)
	if !strings.Contains(string(b), `"query": ""`) || !strings.Contains(string(b), `"path": "/p"`) {
		t.Errorf("bare query or path not emitted:\n%s", b)
	}
}
