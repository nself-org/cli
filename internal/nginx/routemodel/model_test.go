package routemodel

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestBuildAndSetSSL covers the provider contract through HTTP and TLS phases.
func TestBuildAndSetSSL(t *testing.T) {
	cfg := &config.Config{ProjectName: "demo", BaseDomain: "example.test"}
	m, err := Build(cfg, t.TempDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.SchemaVersion != "1" || m.Env != "dev" || m.Defaults.MaxBodyBytes != 100*1024*1024 {
		t.Fatalf("bad defaults: %+v", m)
	}
	if len(m.Routes) != 2 || m.Routes[0].ServerNames[0] != "api.example.test" {
		t.Fatalf("bad core routes: %+v", m.Routes)
	}
	if m.Routes[0].Listen.HTTPS || m.Routes[0].TLS != nil || len(m.Routes[0].Locations) != 5 {
		t.Fatalf("bad HTTP route: %+v", m.Routes[0])
	}
	SetSSL(m, true, func(dir string) bool { return dir == "example-test" })
	if !m.DefaultServer.HTTP.RedirectHTTPS || !m.Routes[0].Listen.HTTPS || m.Routes[0].Listen.HTTP ||
		m.Routes[0].TLS == nil || !m.Routes[0].TLS.HasTrustedChain || len(m.Routes[0].Locations) != 4 {
		t.Fatalf("bad TLS route: %+v", m.Routes[0])
	}
}

// TestBuildRejectsInvalidInput checks malformed body sizes and unsafe targets.
func TestBuildRejectsInvalidInput(t *testing.T) {
	cfg := &config.Config{BaseDomain: "example.test"}
	cfg.Nginx.MaxBody = "many"
	if _, err := Build(cfg, t.TempDir(), false, nil); err == nil {
		t.Fatal("invalid body size accepted")
	}
	cfg.Nginx.MaxBody = "1M"
	for _, target := range []string{"http://a;return 200", "ftp://a", "http:///missing"} {
		cfg.InternalRoutes = []config.InternalRoute{{Index: 1, Name: "bad", Subdomain: "bad", Target: target}}
		if _, err := Build(cfg, t.TempDir(), false, nil); err == nil {
			t.Fatalf("invalid target %q accepted", target)
		}
	}
}

// TestValidateAndMarshal covers duplicate modes and stable output without mutation.
func TestValidateAndMarshal(t *testing.T) {
	m := &Model{Zones: []Zone{{Name: "z"}, {Name: "a"}}, Routes: []Route{
		{ID: "z", ServerNames: []string{"x.test"}, LegacyServerName: "x.test"},
		{ID: "a", ServerNames: []string{"x.test"}, LegacyServerName: "x.test"},
	}}
	if _, err := Validate(m, true); err == nil || !strings.Contains(err.Error(), "E055") {
		t.Fatalf("v1.5 duplicate not refused: %v", err)
	}
	if _, err := Validate(m, false); err == nil || !strings.Contains(err.Error(), "domain conflict") {
		t.Fatalf("legacy duplicate not refused: %v", err)
	}
	first, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Marshal(m)
	if err != nil || !bytes.Equal(first, second) || m.Routes[0].ID != "z" || m.Zones[0].Name != "z" ||
		bytes.Index(first, []byte(`"id": "a"`)) > bytes.Index(first, []byte(`"id": "z"`)) {
		t.Fatalf("marshal changed input or output order: %v", err)
	}
}

// TestParseUpstreamDefaultPorts: a portless target takes its scheme's
// default port, never 0 (review: routes.json and the model-rendered
// proxy_pass would carry ":0").
func TestParseUpstreamDefaultPorts(t *testing.T) {
	for raw, want := range map[string]Upstream{
		"http://example.com":  {Scheme: "http", Host: "example.com", Port: 80},
		"https://example.com": {Scheme: "https", Host: "example.com", Port: 443},
		"hasura:8080":         {Scheme: "http", Host: "hasura", Port: 8080},
		"http://auth:4000/":   {Scheme: "http", Host: "auth", Port: 4000},
		"example.com":         {Scheme: "http", Host: "example.com", Port: 80},
	} {
		if got := parseUpstream(raw); got != want {
			t.Errorf("parseUpstream(%q) = %+v, want %+v", raw, got, want)
		}
	}
}
