package routemodel

// snippet_tls.go — a server's certificate lines as the route's ssl_dir.
//
// Purpose: map the ssl_certificate / ssl_certificate_key pair to ssl_dir.
// Inputs: the certificate directives of one server block.
// Outputs: the dir name, or "" with every certificate line recorded.
// Constraints: a renderer joins ssl_dir into a path, so `.`, `..` and dot
// dirs never match, and a half match (one line elsewhere) maps nothing.
// Only the nested shape maps: the one internal/ssl.Generator writes and the
// CLI's own templates read (/etc/nginx/ssl/certificates/<dir>/, which is what
// a plugin snippet's /etc/nginx/ssl/${SSL_DIR}/ renders to). The older flat
// /etc/nginx/ssl/<dir>/ names a different file than the renderer's
// certificates/<dir>, so it is recorded as unmodelled (D-0571).

import "regexp"

var certRE = regexp.MustCompile(`^/etc/nginx/ssl/certificates/([A-Za-z0-9][A-Za-z0-9._-]*)/(fullchain|privkey)\.pem$`)

// certDir maps exactly one ssl_certificate /etc/nginx/ssl/certificates/<dir>/fullchain.pem
// and one ssl_certificate_key .../<dir>/privkey.pem with the same path to the
// route's ssl_dir <dir>. Anything else (another path or file name, two dirs,
// mixed shapes, a missing half) records every certificate line and leaves
// ssl_dir "".
func (s *srv) certDir() string {
	dirs := map[string]string{}
	ok := len(s.certs) == 2
	for _, d := range s.certs {
		m := certRE.FindStringSubmatch(d.Arg(0))
		want := map[string]string{"ssl_certificate": "fullchain", "ssl_certificate_key": "privkey"}[d.Name]
		if len(d.Args) != 1 || m == nil || m[2] != want {
			ok = false
			continue
		}
		dirs[d.Name] = m[1]
	}
	dir := dirs["ssl_certificate"]
	ok = ok && dir != "" && dir == dirs["ssl_certificate_key"]
	for _, d := range s.certs {
		s.take(d, ok)
	}
	if !ok {
		return ""
	}
	return dir
}
