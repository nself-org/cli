package routemodel

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func testContainer(host string) (string, bool) {
	switch host {
	case "hasura", "auth", "notify":
		return "b_" + host, true
	}
	return "", false
}

func TestQualifyUpstreams(t *testing.T) {
	cfg := &config.Config{ProjectName: "b", BaseDomain: "b.test", Env: "dev"}
	cfg.Hasura.Route = "api"
	cfg.InternalRoutes = []config.InternalRoute{{Name: "svc", Subdomain: "svc", Target: "http://hasura:8080/x", Index: 1}}
	m, err := Build(cfg, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	QualifyUpstreams(m, testContainer)
	QualifyUpstreams(m, testContainer) // idempotent
	got := map[string]string{}
	for _, r := range m.Routes {
		got[r.ID] = r.RawUpstream
		for _, loc := range r.Locations {
			if loc.Path == "/" && loc.Upstream != nil && !strings.HasPrefix(r.RawUpstream, "http://"+loc.Upstream.Host) && !strings.HasPrefix(r.RawUpstream, loc.Upstream.Host) {
				t.Errorf("route %s: location host %q disagrees with RawUpstream %q", r.ID, loc.Upstream.Host, r.RawUpstream)
			}
		}
	}
	if got["core:hasura"] != "b_hasura:8080" || got["core:auth"] != "b_auth:4000" {
		t.Fatalf("core routes not qualified: %v", got)
	}
	if got["internal:1"] != "http://hasura:8080/x" {
		t.Fatalf("an internal route must stay verbatim: %q", got["internal:1"])
	}
	PrefixSiteFiles(m, "b")
	PrefixSiteFiles(m, "b")
	for _, r := range m.Routes {
		if !strings.HasPrefix(r.File, "nginx/sites/b.") || strings.HasPrefix(r.File, "nginx/sites/b.b.") {
			t.Errorf("route %s file %q is not prefixed exactly once", r.ID, r.File)
		}
	}
}

func TestQualifyText(t *testing.T) {
	in := `upstream pool {
    server notify:3709;
    server other:80;
}
server {
    server_name notify.b.test;
    location / {
        set $up http://notify:3709/path;
        proxy_pass http://hasura:8080;
        proxy_pass http://example.com/notify;
        return 301 https://$host$request_uri;
    }
}
`
	out := QualifyText(in, testContainer)
	for _, want := range []string{"server b_notify:3709;", "server other:80;", "http://b_notify:3709/path", "http://b_hasura:8080;", "http://example.com/notify;", "server_name notify.b.test;", "https://$host"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if again := QualifyText(out, testContainer); again != out {
		t.Errorf("QualifyText is not idempotent")
	}
}
