package routemodel

// snippet_access_test.go — return codes, deny all beside allow or satisfy,
// and the certificate pair, from the PR #620 adversarial review.

import "testing"

func TestSnippetReturnRedirectCodes(t *testing.T) {
	for _, code := range []string{"301", "302", "303", "307", "308"} {
		l := route(t, "server { listen 80; server_name a.test; location / { return "+code+" /x; } }").Locations[0]
		if l.Return == nil || l.Return.To == nil || *l.Return.To != "/x" {
			t.Errorf("return %s /x: %+v", code, l.Return)
		}
	}
	for _, code := range []string{"300", "304", "305", "306"} { // nginx sends "/x" as the body, no Location
		d := "return " + code + " /x;"
		r := route(t, "server { listen 80; server_name a.test; location / { "+d+" } }")
		if r.Locations[0].Return != nil || !contains(r.Unmodelled, d) {
			t.Errorf("%s: return=%+v unmodelled=%q", d, r.Locations[0].Return, r.Unmodelled)
		}
	}
}

func TestSnippetDenyAllNeedsNoWayIn(t *testing.T) {
	for name, text := range map[string]string{
		"allow":          `server { listen 80; server_name a.test; location /admin { allow 10.0.0.0/8; deny all; } }`,
		"satisfy any":    `server { listen 80; server_name a.test; location /admin { satisfy any; auth_basic x; deny all; } }`,
		"server satisfy": `server { listen 80; server_name a.test; satisfy any; location /admin { auth_basic x; deny all; } }`,
		"limit_except":   `server { listen 80; server_name a.test; location /le { deny all; limit_except GET { allow all; } proxy_pass http://127.0.0.1:8001; } }`,
		"if allow":       `server { listen 80; server_name a.test; location /admin { if ($x) { allow 1.1.1.1; } deny all; } }`,
		"global satisfy": `satisfy any; server { listen 80; server_name a.test; location /sa { auth_basic x; auth_basic_user_file /f; deny all; } }`,
	} {
		sn, err := ParseSnippet("plugin", "p", "f.conf", text)
		if err != nil || len(sn.Routes) != 1 {
			t.Fatalf("%s: %v", name, err)
		}
		r := sn.Routes[0]
		if r.Locations[0].DenyAll || !contains(r.Unmodelled, "deny all;") {
			t.Errorf("%s: deny_all=%v unmodelled=%q", name, r.Locations[0].DenyAll, r.Unmodelled)
		}
	}
	// A nested location's allow is its own scope.
	if !route(t, `server { listen 80; server_name a.test; location /a { deny all; location /a/b { allow 1.1.1.1; } } }`).Locations[0].DenyAll {
		t.Error("nested location allow blocked deny all")
	}
	// A server-level allow is not inherited by a location with its own deny.
	if !route(t, `server { listen 80; server_name a.test; allow 10.0.0.0/8; satisfy all; location /admin { deny all; } }`).Locations[0].DenyAll {
		t.Error("server allow blocked a location deny all")
	}
}

func TestSnippetCertPair(t *testing.T) {
	const pre = "server { listen 443 ssl; server_name a.test; "
	for name, certs := range map[string][]string{
		"two dirs":    {"ssl_certificate /etc/nginx/ssl/a/fullchain.pem;", "ssl_certificate_key /etc/nginx/ssl/b/privkey.pem;"},
		"data cert":   {"ssl_certificate data:x;", "ssl_certificate_key /etc/nginx/ssl/a/privkey.pem;"},
		"cert only":   {"ssl_certificate /etc/nginx/ssl/a/fullchain.pem;"},
		"second cert": {"ssl_certificate /etc/nginx/ssl/a/fullchain.pem;", "ssl_certificate_key /etc/nginx/ssl/a/privkey.pem;", "ssl_certificate /etc/nginx/ssl/a/fullchain.pem;"},
	} {
		text := pre
		for _, c := range certs {
			text += c + " "
		}
		r := route(t, text+"}")
		if r.TLS == nil || r.TLS.SSLDir != "" {
			t.Errorf("%s: tls=%+v", name, r.TLS)
		}
		for _, c := range certs {
			if !contains(r.Unmodelled, c) {
				t.Errorf("%s: %q not recorded in %q", name, c, r.Unmodelled)
			}
		}
	}
}
