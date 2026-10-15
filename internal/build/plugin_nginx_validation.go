package build

import (
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/nginx/routemodel"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// placeholderRE finds braced names; only buildTemplateVars keys are templates.
// Other names may be native nginx variables, including uppercase map outputs.
var placeholderRE = regexp.MustCompile(`\$\{[A-Z][A-Z0-9_]*\}`)

// serverOnly lists directives nginx accepts only inside server (or deeper).
var serverOnly = map[string]bool{"location": true, "if": true, "listen": true, "server_name": true,
	"proxy_pass": true, "fastcgi_pass": true, "uwsgi_pass": true, "scgi_pass": true, "grpc_pass": true,
	"return": true, "rewrite": true, "try_files": true, "set": true, "alias": true, "internal": true, "limit_except": true}

// Purpose: exclude snippets known to prevent nginx from starting.
// Inputs: rendered snippet, template values, and the shared core/plugin TLS gate.
// Outputs: a skip reason, or an empty string when the snippet can be injected.
// Constraints: inspect parsed directives to ignore comments without discarding
// quoted text; native variable validity remains nginx's responsibility.
func pluginSnippetSkip(rendered string, vars map[string]string, hasSSL bool) string {
	tree, err := routemodel.Parse(rendered)
	if err != nil {
		return "it does not parse: " + err.Error()
	}
	var directives strings.Builder
	for _, b := range tree {
		if serverOnly[b.Name] {
			return "it has a top-level " + b.Name + " directive, which nginx accepts only inside a server block"
		}
		directives.WriteString(b.Render())
		directives.WriteByte('\n')
	}
	text := directives.String()
	for _, m := range placeholderRE.FindAllString(text, -1) {
		if _, known := vars[m[2:len(m)-1]]; known {
			return "it uses " + m + ", which nself has not rendered"
		}
	}
	if !hasSSL && strings.Contains(text, nginxtopo.NginxSSLContainerPath+"/"+vars["SSL_DIR"]+"/") {
		return "it needs the base domain's TLS certificate, which this build does not serve (issue it with nself trust ssl setup, or set SSL_MODE=local)"
	}
	return ""
}
