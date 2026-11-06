package plugincheck

import "testing"

func TestLoopbackGated(t *testing.T) {
	const gate = "allow 127.0.0.1; allow ::1; deny all; "
	cases := []struct {
		name, text string
		want       bool
	}{
		{"gated", "server { listen 443; server_name a.x; " + gate + "location /v1/ { proxy_pass http://a:1; } }", true},
		{"gated, deny first", "server { listen 443; deny all; allow ::1; location / { proxy_pass http://a:1; } }", true},
		{"two servers, both gated", "server { listen 80; " + gate + "} server { listen 443; " + gate + "}", true},
		{"open", "server { listen 443; server_name a.x; location / { proxy_pass http://a:1; } }", false},
		{"one of two open", "server { listen 80; " + gate + "} server { listen 443; }", false},
		{"no server block", "upstream u { server a:1; }", false},
		{"private range allowed", "server { allow 10.0.0.0/8; deny all; }", false},
		{"allow all", "server { allow all; deny all; }", false},
		{"location allow drops the gate", "server { " + gate + "location / { allow all; proxy_pass http://a:1; } }", false},
		{"location deny drops the gate", "server { " + gate + "location / { deny 1.2.3.4; } }", false},
		{"satisfy any", "server { satisfy any; " + gate + "}", false},
		{"return answers first", "server { " + gate + "location / { return 200 ok; } }", false},
		{"real_ip trusts a header", "server { " + gate + "set_real_ip_from 0.0.0.0/0; real_ip_header X-Forwarded-For; }", false},
		{"top-level error_page", "error_page 403 = /ok; server { " + gate + "}", false},
		{"does not parse", "server { deny all;", false},
	}
	for _, c := range cases {
		if got := LoopbackGated(c.text); got != c.want {
			t.Errorf("%s: LoopbackGated = %v, want %v", c.name, got, c.want)
		}
	}
}
