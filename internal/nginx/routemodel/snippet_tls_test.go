package routemodel

// snippet_tls_test.go — D-0505: the nested certificate path internal/ssl
// writes (and a plugin's /etc/nginx/ssl/${SSL_DIR}/ renders to) maps to the
// route's ssl_dir, the flat shape stays unmodelled, mixed shapes do not.

import (
	"reflect"
	"testing"
)

func TestSnippetCertDirShapes(t *testing.T) {
	cases := []struct {
		name, cert, key, want string
	}{
		{"nested", "/etc/nginx/ssl/certificates/local-nself-org/fullchain.pem", "/etc/nginx/ssl/certificates/local-nself-org/privkey.pem", "local-nself-org"},
		{"flat", "/etc/nginx/ssl/local-nself-org/fullchain.pem", "/etc/nginx/ssl/local-nself-org/privkey.pem", ""},
		{"flat dir named certificates", "/etc/nginx/ssl/certificates/fullchain.pem", "/etc/nginx/ssl/certificates/privkey.pem", ""},
		{"mixed shapes", "/etc/nginx/ssl/certificates/a/fullchain.pem", "/etc/nginx/ssl/a/privkey.pem", ""},
		{"nested two dirs", "/etc/nginx/ssl/certificates/a/fullchain.pem", "/etc/nginx/ssl/certificates/b/privkey.pem", ""},
		{"nested dot dir", "/etc/nginx/ssl/certificates/.x/fullchain.pem", "/etc/nginx/ssl/certificates/.x/privkey.pem", ""},
		{"nested dotdot", "/etc/nginx/ssl/certificates/../fullchain.pem", "/etc/nginx/ssl/certificates/../privkey.pem", ""},
		{"two levels", "/etc/nginx/ssl/certificates/a/b/fullchain.pem", "/etc/nginx/ssl/certificates/a/b/privkey.pem", ""},
		{"other subdir", "/etc/nginx/ssl/live/a/fullchain.pem", "/etc/nginx/ssl/live/a/privkey.pem", ""},
	}
	for _, c := range cases {
		cert := "ssl_certificate " + c.cert + ";"
		key := "ssl_certificate_key " + c.key + ";"
		r := route(t, "server { listen 443 ssl; server_name a.test; "+cert+" "+key+" }")
		if r.TLS == nil || r.TLS.SSLDir != c.want {
			t.Errorf("%s: tls=%+v, want ssl_dir %q", c.name, r.TLS, c.want)
			continue
		}
		var wantRec []string
		if c.want == "" {
			wantRec = []string{cert, key}
		}
		if len(r.Unmodelled) != len(wantRec) || (wantRec != nil && !reflect.DeepEqual(r.Unmodelled, wantRec)) {
			t.Errorf("%s: unmodelled=%q, want %q", c.name, r.Unmodelled, wantRec)
		}
	}
}
