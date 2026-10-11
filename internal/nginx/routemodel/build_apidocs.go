package routemodel

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// apiDocsPathRE is the set of API_DOCS_PATH values safe to write raw into an
// nginx "location <path> {" line.
var apiDocsPathRE = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)

// APIDocs returns the docs.<base> route that nginx/sites/api-docs.conf serves,
// or nil, nil when the docs are off. It mirrors apidocs.NginxConf and the
// build finish step: docs are on when ApiDocs.Enabled is set or the section is
// empty (Path ""), and the docs page is served at ApiDocs.Path ("/docs" by
// default). The route is not part of Build's Model.Routes: nginx renders those,
// and the finish step already writes api-docs.conf, so a second copy would be a
// duplicate server block. The routes.json writer appends it instead.
//
// workdir is used only to flag a hand-managed conf.d file that already serves
// the same server name (ShadowedBy), as makeRoute does for the core routes.
// An API_DOCS_PATH that nginx would reject or that collides with /api-docs is
// an error.
func APIDocs(cfg *config.Config, workdir string) (*Route, error) {
	if !cfg.ApiDocs.Enabled && cfg.ApiDocs.Path != "" {
		return nil, nil
	}
	path := cfg.ApiDocs.Path
	if path == "" {
		path = "/docs"
	}
	if !apiDocsPathRE.MatchString(path) {
		return nil, fmt.Errorf("API_DOCS_PATH %q: must start with / and contain only letters, digits and . _ ~ / -", path)
	}
	if strings.HasPrefix(path, "/api-docs") {
		return nil, fmt.Errorf("API_DOCS_PATH %q: must not equal or start with /api-docs (nginx rejects the duplicate location)", path)
	}
	base := cfg.BaseDomain
	if base == "" {
		base = "localhost"
	}
	name := "docs." + base
	// Listen/TLS are fixed: apidocs.NginxConf always writes "listen 443 ssl",
	// http2 and the certificate paths. http2 is ignored by the snippet parser
	// and server_tokens off is the global default (Defaults.ServerTokens), so
	// nothing is left unmodelled.
	return &Route{
		ID: "core:api-docs", Source: "core", File: "nginx/sites/api-docs.conf",
		ServerNames: []string{name}, Listen: Listen{HTTP: false, HTTPS: true},
		TLS: &RouteTLS{SSLDir: strings.ReplaceAll(base, ".", "-"),
			Protocols: []string{"TLSv1.2", "TLSv1.3"}, Ciphers: ptr(ciphers)},
		SecurityHeaders: map[string]string{}, BlockedPaths: []BlockedPath{},
		Locations: []Location{
			{Path: "/api-docs", Match: "prefix", StaticFile: ptr("/opt/nself/.dist/openapi.json"),
				HeadersAdd: map[string]string{"Content-Type": "application/json",
					"Access-Control-Allow-Origin": "*", "Cache-Control": "no-store"},
				HeadersSet: map[string]string{}, Timeouts: Timeouts{}, AccessLog: true},
			{Path: path, Match: "prefix", StaticFile: ptr("/opt/nself/.dist/scalar.html"),
				HeadersAdd: map[string]string{"Content-Type": "text/html", "Cache-Control": "no-store"},
				HeadersSet: map[string]string{}, Timeouts: Timeouts{}, AccessLog: true},
		},
		ShadowedBy: shadowFile(cfg, workdir, name), Unmodelled: []string{},
	}, nil
}
