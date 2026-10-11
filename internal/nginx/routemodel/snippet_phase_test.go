package routemodel

// snippet_phase_test.go — nginx phase order, HTTP version and certificate path
// cases from the PR #604 post-merge review: `set` runs before proxy_pass
// wherever it sits, a websocket needs proxy_http_version 1.1, and ssl_dir is
// never `.`, `..` or a dot directory.

import (
	"reflect"
	"testing"
)

func TestSnippetSetRunsBeforeProxyPass(t *testing.T) {
	// The location's own set wins over the server's, even written after proxy_pass.
	r := route(t, `server{listen 80;server_name a.example.com;set $up http://a.example:80;location /{proxy_pass $up;set $up http://b.example:81;}}`)
	if u := r.Locations[0].Upstream; u == nil || *u != (Upstream{"http", "b.example", 81}) {
		t.Errorf("upstream = %+v, want b.example:81", u)
	}
	if !contains(r.Unmodelled, "set $up http://a.example:80;") || contains(r.Unmodelled, "set $up http://b.example:81;") {
		t.Errorf("unmodelled = %q", r.Unmodelled)
	}
	// A variable set twice in one scope is not resolved: proxy_pass and both sets are recorded.
	for name, text := range map[string]string{
		"location": `server{listen 80;server_name a.test;location /{set $up http://a.test:1;proxy_pass $up;set $up http://b.test:2;}}`,
		"server":   `server{listen 80;server_name a.test;set $up http://a.test:1;set $up http://b.test:2;location /{proxy_pass $up;}}`,
	} {
		r := route(t, text)
		want := []string{"set $up http://a.test:1;", "set $up http://b.test:2;", "proxy_pass $up;"}
		for _, w := range want {
			if !contains(r.Unmodelled, w) {
				t.Errorf("%s: %q not recorded in %q", name, w, r.Unmodelled)
			}
		}
		if r.Locations[0].Upstream != nil {
			t.Errorf("%s: upstream mapped to %+v", name, r.Locations[0].Upstream)
		}
	}
	// An if, rewrite or break in the server or location may skip or change a set.
	for name, text := range map[string]string{
		"location if":      `server{listen 80;server_name a.test;set $up http://a.test:1;location /{if ($arg_x) {set $up http://b.test:2;} proxy_pass $up;}}`,
		"location break":   `server{listen 80;server_name a.test;location /{break;set $up http://a.test:1;proxy_pass $up;}}`,
		"location rewrite": `server{listen 80;server_name a.test;location /{rewrite ^ /x break;set $up http://a.test:1;proxy_pass $up;}}`,
		"server if":        `server{listen 80;server_name a.test;set $up http://a.test:1;if ($host = b) {set $up http://b.test:2;} location /{proxy_pass $up;}}`,
	} {
		r := route(t, text)
		if r.Locations[0].Upstream != nil || !contains(r.Unmodelled, "proxy_pass $up;") {
			t.Errorf("%s: upstream=%+v unmodelled=%q", name, r.Locations[0].Upstream, r.Unmodelled)
		}
	}
	// nginx variable names are case-insensitive.
	r = route(t, `server{listen 80;server_name a.test;set $up http://a.test:1;location /{set $UP http://b.test:2;proxy_pass $up;}}`)
	if u := r.Locations[0].Upstream; u == nil || *u != (Upstream{"http", "b.test", 2}) {
		t.Errorf("case-folded location set: upstream = %+v", u)
	}
	// A second set differing only in case, or another writer of the variable
	// (auth_request_set or set_by_lua in scope, map or js_set at the top of the
	// file), may change it: the proxy_pass is recorded.
	for _, c := range []struct{ name, pass, text string }{
		{"location case dup", "$v", `server{listen 80;server_name a.test;location /{set $v http://a.test:1;set $V http://b.test:2;proxy_pass $v;}}`},
		{"server case dup", "$up", `server{listen 80;server_name a.test;set $up http://a.test:1;set $Up http://b.test:2;location /{proxy_pass $up;}}`},
		{"auth_request_set", "$w", `server{listen 80;server_name a.test;location /{set $w http://a.test:1;auth_request /auth;auth_request_set $w $upstream_http_x;proxy_pass $w;}}`},
		{"server writer", "$w", `server{listen 80;server_name a.test;set_by_lua $W 'return 1';location /{set $w http://a.test:1;proxy_pass $w;}}`},
		{"global map writer", "$w", `map $host $w { default http://b.test:2; } server{listen 80;server_name a.test;location /{set $w http://a.test:1;proxy_pass $w;}}`},
		{"global js_set writer", "$w", `js_set $w m.f; server{listen 80;server_name a.test;location /{set $w http://a.test:1;proxy_pass $w;}}`},
	} {
		r := route(t, c.text)
		if r.Locations[0].Upstream != nil || !contains(r.Unmodelled, "proxy_pass "+c.pass+";") {
			t.Errorf("%s: upstream=%+v unmodelled=%q", c.name, r.Locations[0].Upstream, r.Unmodelled)
		}
	}
	// Without a variable, an if elsewhere does not block a literal upstream.
	if u := route(t, `server{listen 80;server_name a.test;if ($host = b) {return 404;} location /{proxy_pass http://a.test:1;}}`).Locations[0].Upstream; u == nil {
		t.Error("literal upstream refused")
	}
}

func TestSnippetWebSocketNeedsHTTP11(t *testing.T) {
	const ws = `proxy_pass http://x.test:81; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade";`
	cases := []struct {
		name, server, loc string
		want              bool
		rec               []string
	}{
		{"1.1 in location", "", "proxy_http_version 1.1; " + ws, true, nil},
		{"1.1 inherited", "proxy_http_version 1.1; ", ws, true, nil},
		{"1.0", "", "proxy_http_version 1.0; " + ws, false,
			[]string{"proxy_http_version 1.0;", "proxy_set_header Upgrade $http_upgrade;", `proxy_set_header Connection "upgrade";`}},
		{"absent", "", ws, false, []string{"proxy_set_header Upgrade $http_upgrade;", `proxy_set_header Connection "upgrade";`}},
		{"location 1.0 over server 1.1", "proxy_http_version 1.1; ", "proxy_http_version 1.0; " + ws, false,
			[]string{"proxy_http_version 1.0;", "proxy_set_header Upgrade $http_upgrade;"}},
	}
	for _, c := range cases {
		r := route(t, "server { listen 80; server_name a.test; "+c.server+"location / { "+c.loc+" } }")
		l := r.Locations[0]
		if l.WebSocket != c.want || len(l.HeadersSet) != 0 {
			t.Errorf("%s: websocket=%v headers=%v", c.name, l.WebSocket, l.HeadersSet)
		}
		for _, w := range c.rec {
			if !contains(r.Unmodelled, w) {
				t.Errorf("%s: %q not recorded in %q", c.name, w, r.Unmodelled)
			}
		}
		if c.rec == nil && len(r.Unmodelled) != 0 {
			t.Errorf("%s: recorded %q", c.name, r.Unmodelled)
		}
	}
}

func TestSnippetCertDirRejectsDots(t *testing.T) {
	for _, dir := range []string{".", "..", ".hidden", "-x"} {
		cert := "ssl_certificate /etc/nginx/ssl/" + dir + "/fullchain.pem;"
		key := "ssl_certificate_key /etc/nginx/ssl/" + dir + "/privkey.pem;"
		r := route(t, "server { listen 443 ssl; server_name a.test; "+cert+" "+key+" }")
		if r.TLS == nil || r.TLS.SSLDir != "" || !reflect.DeepEqual(r.Unmodelled, []string{cert, key}) {
			t.Errorf("%s: tls=%+v unmodelled=%q", dir, r.TLS, r.Unmodelled)
		}
	}
	r := route(t, "server { listen 443 ssl; server_name a.test; ssl_certificate /etc/nginx/ssl/a.test_1/fullchain.pem; ssl_certificate_key /etc/nginx/ssl/a.test_1/privkey.pem; }")
	if r.TLS == nil || r.TLS.SSLDir != "a.test_1" || len(r.Unmodelled) != 0 {
		t.Errorf("valid dir: tls=%+v unmodelled=%q", r.TLS, r.Unmodelled)
	}
}
