// Package plugincheck validates rendered plugin nginx snippets before installation.
package plugincheck

import (
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/nginx/routemodel"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// placeholderRE finds template-shaped variables; the parsed directive and
// known template vocabulary distinguish them from comments and nginx variables.
var placeholderRE = regexp.MustCompile(`\$\{[A-Z][A-Z0-9_]*\}`)

// serverOnly lists directives nginx accepts only inside server (or deeper):
// at the http{} level nginx/sites is included in, each one stops nginx with
// "directive is not allowed here".
var serverOnly = map[string]bool{"location": true, "if": true, "listen": true, "server_name": true,
	"proxy_pass": true, "fastcgi_pass": true, "uwsgi_pass": true, "scgi_pass": true, "grpc_pass": true,
	"return": true, "rewrite": true, "try_files": true, "set": true, "alias": true, "internal": true, "limit_except": true}

// Skip returns why a rendered plugin snippet must stay out of
// nginx/sites, or "" to inject it. One bad file in nginx/sites stops nginx,
// taking every route down, and installed plugin copies can be old, so the
// build checks what it can before nginx does (D-0478 review):
//   - a known template name that remains unrendered in directive arguments;
//   - text that does not parse, or a top-level directive that is valid only
//     inside a server block (a bare location block);
//   - a load of the base domain's certificate pair (the ${SSL_DIR} path) while
//     the build serves no TLS (hasSSL false: SSL_MODE none, or letsencrypt/
//     custom before the pair exists), which would also block the ACME
//     challenge that issues it.
func Skip(rendered string, vars map[string]string, hasSSL bool) string {
	tree, err := routemodel.Parse(rendered)
	if err != nil {
		return "it does not parse: " + err.Error()
	}
	for _, b := range tree {
		for _, m := range placeholderRE.FindAllString(b.Render(), -1) {
			name := strings.TrimSuffix(strings.TrimPrefix(m, "${"), "}")
			_, known := vars[name]
			if known || name == "DOMAIN" || name == "NSELF_DOMAIN" || name == "GRAFANA_ROUTE" {
				return "it uses " + m + ", which nself does not provide"
			}
		}
	}
	for _, b := range tree {
		if serverOnly[b.Name] {
			return "it has a top-level " + b.Name + " directive, which nginx accepts only inside a server block"
		}
	}
	if !hasSSL && strings.Contains(rendered, nginxtopo.NginxSSLContainerPath+"/"+vars["SSL_DIR"]+"/") {
		return "it needs the base domain's TLS certificate, which this build does not serve (issue it with nself trust ssl setup, or set SSL_MODE=local)"
	}
	return ""
}
