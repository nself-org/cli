package plugincheck

import (
	"strings"
	"testing"
)

func TestPluginSnippetSkip(t *testing.T) {
	vars := map[string]string{"SSL_DIR": "certificates/example-com", "BASE_DOMAIN": "example.com"}
	cert := "    ssl_certificate /etc/nginx/ssl/certificates/example-com/fullchain.pem;\n"
	cases := []struct {
		name, text string
		hasSSL     bool
		want       string // substring of the reason; "" = injected
	}{
		{"comment placeholder", "# ${NSELF_DOMAIN}\nserver { listen 80; }\n", false, ""},
		{"native uppercase variable", "map $host $UPSTREAM { default localhost; }\nserver { location / { proxy_pass http://${UPSTREAM}; } }\n", false, ""},
		{"native route variable", "map $host $CUSTOM_ROUTE { default localhost; }\nserver { location / { proxy_pass http://${CUSTOM_ROUTE}; } }\n", false, ""},
		{"valid server block", "server {\n    listen 80;\n    server_name a.example.com;\n    location / { return 204; }\n}\n", false, ""},
		{"nginx variable in braces", "server {\n    listen 80;\n    location / { return 301 https://${host}$request_uri; }\n}\n", false, ""},
		{"upstream and map at top level", "upstream u { server a:1; }\nmap $http_upgrade $c { default upgrade; }\nserver { listen 80; }\n", false, ""},
		{"unrendered placeholder", "server {\n    listen 443 ssl;\n    ssl_certificate /etc/nginx/ssl/${NSELF_DOMAIN}/fullchain.pem;\n}\n", true, "${NSELF_DOMAIN}"},
		{"unrendered route var", "server {\n    server_name ${GRAFANA_ROUTE}.example.com;\n}\n", true, "${GRAFANA_ROUTE}"},
		{"bare location block", "location /plugins/documents/ {\n    proxy_pass http://documents:3000;\n}\n", true, "top-level location"},
		{"does not parse", "server { listen 80;", true, "does not parse"},
		{"top-level set", "set $upstream http://a:1;\nserver { listen 80; }\n", true, "top-level set"},
		{"top-level cert pair is legal at http level", "ssl_certificate /etc/nginx/ssl/other/fullchain.pem;\nssl_certificate_key /etc/nginx/ssl/other/privkey.pem;\nserver { listen 80; }\n", true, ""},
		{"base cert, TLS served", "server {\n    listen 443 ssl;\n" + cert + "}\n", true, ""},
		{"base cert, no TLS served", "server {\n    listen 443 ssl;\n" + cert + "}\n", false, "TLS certificate"},
	}
	for _, c := range cases {
		got := Skip(c.text, vars, c.hasSSL)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: pluginSnippetSkip = %q, want reason containing %q", c.name, got, c.want)
		}
	}
}
