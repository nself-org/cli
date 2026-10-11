package routemodel

// Purpose: tie the api-docs route to the real apidocs.NginxConf output so the
//          two cannot drift apart (D-0310).
// Inputs:  apidocs.NginxConf rendered for the default and a custom path.
// Outputs: none (t.Error on assertion failure).
// Note:    apidocs imports only nginxtopo, so there is no import cycle. The
//          snippet parser leaves alias and add_header unmodelled, so the conf
//          is read with Tokenize.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/apidocs"
	"github.com/nself-org/cli/internal/config"
)

// TestAPIDocsMatchesNginxConf compares server_name, listen, cert dir, each
// location path, its alias target and its add_header set with APIDocs.
func TestAPIDocsMatchesNginxConf(t *testing.T) {
	for _, tc := range []struct{ name, path, base string }{
		{"default", "/docs", "example.test"},
		{"custom path", "/reference", "my.example.test"},
		{"empty base", "/docs", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{BaseDomain: tc.base,
				ApiDocs: config.ApiDocsConfig{Enabled: true, Path: tc.path}}
			got, err := APIDocs(cfg)
			if err != nil || got == nil {
				t.Fatalf("APIDocs = %v, %v", got, err)
			}
			text := apidocs.NginxConf(tc.path, tc.base)
			tree, perr := Parse(text)
			if perr != nil {
				t.Fatalf("Parse: %v", perr)
			}
			var srv *Block
			for _, b := range tree {
				if b.Name == "server" && b.Block {
					srv = b
				}
			}
			if srv == nil {
				t.Fatalf("no server block in:\n%s", text)
			}
			var names, listens, certs []string
			type loc struct {
				path, alias string
				headers     map[string]string
			}
			var locs []loc
			for _, d := range srv.Children {
				switch {
				case d.Name == "server_name":
					names = append(names, d.Args...)
				case d.Name == "listen":
					listens = append(listens, strings.Join(d.Args, " "))
				case d.Name == "ssl_certificate":
					certs = append(certs, Unquote(d.Arg(0)))
				case d.Name == "location" && d.Block:
					l := loc{path: Unquote(d.Arg(0)), headers: map[string]string{}}
					for _, c := range d.Children {
						switch c.Name {
						case "alias":
							l.alias = Unquote(c.Arg(0))
						case "add_header":
							l.headers[Unquote(c.Arg(0))] = Unquote(c.Arg(1))
						}
					}
					locs = append(locs, l)
				}
			}
			if !reflect.DeepEqual(got.ServerNames, names) {
				t.Errorf("server_names = %v, conf has %v", got.ServerNames, names)
			}
			// Only 443 ssl is listened on (IPv4 and IPv6), never port 80.
			for _, l := range listens {
				if !strings.Contains(l, "443") || strings.Contains(l, ":80") || l == "80" {
					t.Errorf("conf listens on %q, route models 443 only", l)
				}
			}
			if got.Listen.HTTP || !got.Listen.HTTPS || len(listens) == 0 {
				t.Errorf("route listen = %+v, conf listens %v", got.Listen, listens)
			}
			if len(certs) != 1 || got.TLS == nil ||
				!strings.Contains(certs[0], "/certificates/"+got.TLS.SSLDir+"/fullchain.pem") {
				t.Errorf("cert %v does not match ssl_dir %+v", certs, got.TLS)
			}
			if len(got.Locations) != len(locs) {
				t.Fatalf("locations = %d, conf has %d", len(got.Locations), len(locs))
			}
			for i, g := range got.Locations {
				w := locs[i]
				if g.Path != w.path || g.Match != "prefix" {
					t.Errorf("location %d = %s %q, conf has %q", i, g.Match, g.Path, w.path)
				}
				if g.StaticFile == nil || *g.StaticFile != w.alias {
					t.Errorf("location %q static_file = %v, conf alias %q", g.Path, g.StaticFile, w.alias)
				}
				if !reflect.DeepEqual(g.HeadersAdd, w.headers) {
					t.Errorf("location %q headers_add = %v, conf has %v", g.Path, g.HeadersAdd, w.headers)
				}
			}
		})
	}
}
