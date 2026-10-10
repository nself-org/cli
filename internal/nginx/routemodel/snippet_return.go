package routemodel

// snippet_return.go — `return` directives in plugin snippets and conf.d files.
//
// Purpose: map `return CODE;` and `return 30x URL;` (server level or location
// level) to the model's return field and http_to_https_redirect.
// Inputs: one return directive.
// Outputs: Return{Status, To}; a server-level return becomes the "/" location.
// Constraints: a body (`return 200 "ok"`), a bare URL and a URL with an nginx
// variable other than the canonical https redirect are recorded.

import (
	"strconv"
	"strings"
)

// redirectHTTPS is the one `return` target that sets http_to_https_redirect.
const redirectHTTPS = "https://$host$request_uri"

func isHTTPSRedirect(r *Return) bool {
	switch r.Status {
	case 301, 302, 307, 308:
		return r.To != nil && *r.To == redirectHTTPS
	}
	return false
}

// serverReturn maps a server-level return. With no location in the server it
// becomes the "/" location; otherwise it pre-empts every location, which the
// model cannot say, so it is recorded.
func (s *srv) serverReturn(d *Block, nLoc int) {
	ret, ok, lossy := parseReturn(d)
	if !s.take(d, ok && nLoc == 0) {
		return
	}
	s.r.Locations = append(s.r.Locations, Location{Path: "/", Match: "prefix", HeadersSet: map[string]string{}, AccessLog: true, Return: ret})
	s.r.HTTPToHTTPSRedirect = isHTTPSRedirect(ret)
	if lossy {
		s.unm(d)
	}
}

// parseReturn reads `return CODE;` and `return 30x URL;`. lossy is true when
// the URL holds an nginx variable other than the canonical https redirect, so
// the directive is also recorded. A body (`return 200 "ok"`) or a bare URL is
// refused.
func parseReturn(d *Block) (ret *Return, ok, lossy bool) {
	code, err := strconv.Atoi(d.Arg(0))
	if d.Block || len(d.Args) < 1 || len(d.Args) > 2 || err != nil || code < 100 || code > 599 {
		return nil, false, false
	}
	ret = &Return{Status: code}
	if len(d.Args) == 1 {
		return ret, true, false
	}
	to := d.Arg(1)
	if code < 301 || code > 308 || !strings.HasPrefix(to, "/") && !strings.HasPrefix(to, "http://") && !strings.HasPrefix(to, "https://") {
		return nil, false, false
	}
	ret.To = &to
	return ret, true, strings.Contains(to, "$") && !isHTTPSRedirect(ret)
}
