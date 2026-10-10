package routemodel

// snippet_map.go — the directive table for locations (EPIC DEPL D2).
//
// Purpose: map each directive inside a `location` to a model field, to "no
// observable effect", or to the route's `unmodelled`.
// Inputs: the directives of one location, the server-level directives it
// inherits (timeouts, body size, access_log, proxy_set_header) and the `set`
// variables in scope.
// Outputs: one Location per mappable `location` block.
// Constraints: a value that cannot be represented exactly is recorded, never
// rounded. A proxy_pass with a URI part keeps its upstream AND is recorded (v1
// cannot carry the path). Regex, named and ^~ locations, nested locations,
// `if`, `client_max_body_size 0`, a `deny` other than `all`, a sub-second
// timeout and any directive outside the table are recorded.

import (
	"net"
	"regexp"
	"strconv"
	"strings"
)

// redirectHTTPS is the one `return` target that sets http_to_https_redirect.
const redirectHTTPS = "https://$host$request_uri"

var (
	identRE = regexp.MustCompile(`^[A-Za-z0-9_]+`)
	hdrRE   = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	methRE  = regexp.MustCompile(`^[A-Z]+$`)
)

// stdHeaders are the four forwarded headers that forwarded_headers stands for.
var stdHeaders = map[string]string{"host": "$host", "x-real-ip": "$remote_addr",
	"x-forwarded-for": "$proxy_add_x_forwarded_for", "x-forwarded-proto": "$scheme"}

type hdr struct {
	name, val string
	node      *Block
}

// locAcc accumulates one location.
type locAcc struct {
	s     *srv
	loc   Location
	vars  map[string]*setVar
	local []*setVar
	hdrs  []hdr
	done  map[string]bool // single-valued directives already seen
}

func isHTTPSRedirect(r *Return) bool {
	switch r.Status {
	case 301, 302, 307, 308:
		return r.To != nil && *r.To == redirectHTTPS
	}
	return false
}

// location maps one `location` block; the rest are recorded whole.
func (s *srv) location(l *Block) {
	a := unquoteAll(l.Args)
	match, path := "", ""
	switch {
	case len(a) == 1 && strings.HasPrefix(a[0], "/"):
		match, path = "prefix", a[0]
	case len(a) == 2 && a[0] == "=" && strings.HasPrefix(a[1], "/"):
		match, path = "exact", a[1]
	}
	for _, e := range s.r.Locations {
		if e.Path == path && e.Match == match {
			match = ""
		}
	}
	if !s.take(l, match != "" && l.Block) {
		return
	}
	s.sn.disp[l] = "container"
	acc := &locAcc{s: s, vars: map[string]*setVar{}, done: map[string]bool{},
		loc: Location{Path: path, Match: match, HeadersSet: map[string]string{}, AccessLog: true}}
	for k, v := range s.vars {
		acc.vars[k] = v
	}
	own := map[string]bool{}
	for _, d := range l.Children {
		own[d.Name] = true
	}
	for _, n := range s.inh {
		if !own[n.Name] {
			acc.apply(n)
		}
	}
	for _, d := range l.Children {
		acc.apply(d)
	}
	acc.finish()
	s.r.Locations = append(s.r.Locations, acc.loc)
}

// take marks d mapped when ok and it is the first of its name, else records it.
func (a *locAcc) take(d *Block, ok bool) bool {
	if a.done[d.Name] && ok {
		ok = false
	}
	a.done[d.Name] = a.done[d.Name] || ok
	return a.s.take(d, ok)
}

// apply handles one directive of the location (or one inherited from the server).
func (a *locAcc) apply(d *Block) {
	switch d.Name {
	case "proxy_http_version", "proxy_buffer_size", "proxy_buffers", "proxy_busy_buffers_size", "resolver":
		a.s.sn.disp[d] = "ignored"
	case "proxy_pass":
		a.proxyPass(d)
	case "proxy_set_header":
		if a.s.take(d, len(d.Args) == 2 && hdrRE.MatchString(d.Arg(0))) {
			a.hdrs = append(a.hdrs, hdr{d.Arg(0), d.Arg(1), d})
			a.s.sn.disp[d] = "" // claimed in finish
		}
	case "proxy_connect_timeout", "proxy_read_timeout", "proxy_send_timeout":
		secs, ok := parseSeconds(d)
		t := &a.loc.Timeouts
		if a.take(d, ok) {
			*map[string]**int{"proxy_connect_timeout": &t.ConnectS, "proxy_read_timeout": &t.ReadS, "proxy_send_timeout": &t.SendS}[d.Name] = &secs
		}
	case "client_max_body_size":
		n, ok := parseBodySize(d)
		if a.take(d, ok) {
			a.loc.MaxBodyBytes = &n
		}
	case "access_log":
		a.loc.AccessLog = !a.take(d, len(d.Args) == 1 && d.Arg(0) == "off")
	case "deny":
		a.loc.DenyAll = a.take(d, len(d.Args) == 1 && d.Arg(0) == "all")
	case "limit_except":
		a.limitExcept(d)
	case "return":
		ret, ok, lossy := parseReturn(d)
		if a.take(d, ok) {
			a.loc.Return = ret
			if lossy {
				a.s.unm(d)
			}
		}
	case "set":
		if len(d.Args) == 2 && strings.HasPrefix(d.Arg(0), "$") {
			v := &setVar{d.Arg(1), d}
			a.vars[d.Arg(0)[1:]] = v
			a.local = append(a.local, v)
			return
		}
		a.s.unm(d)
	default:
		a.s.unm(d)
	}
}

// limitExcept maps `limit_except M... { deny all; }` to methods.
func (a *locAcc) limitExcept(d *Block) {
	ok := d.Block && len(d.Args) > 0 && len(d.Children) == 1
	if ok {
		c := d.Children[0]
		ok = c.Name == "deny" && len(c.Args) == 1 && c.Arg(0) == "all" && !c.Block
		a.s.sn.disp[c] = "mapped"
	}
	var ms []string
	for _, m := range unquoteAll(d.Args) {
		ok = ok && methRE.MatchString(m)
		ms = appendUnique(ms, m)
	}
	if a.take(d, ok) {
		a.loc.Methods = ms
	}
}

// proxyPass resolves the target to scheme, host and port. A `set` variable is
// substituted literally when it is the only variable and is in scope; an
// upstream name resolves through upstream{}; a portless name that is neither
// an upstream of this file, dotted nor an IP could be an upstream defined in
// another file, so it is recorded.
func (a *locAcc) proxyPass(d *Block) {
	raw := d.Arg(0)
	var used *setVar
	if i := strings.IndexByte(raw, '$'); i >= 0 {
		name := identRE.FindString(raw[i+1:])
		rest := raw[i+1+len(name):]
		if v := a.vars[name]; v != nil && !strings.ContainsAny(raw[:i]+rest, "$") {
			raw, used = raw[:i]+v.val+rest, v
		}
	}
	scheme, rest, found := strings.Cut(raw, "://")
	hostport, tail := rest, ""
	if k := strings.IndexAny(rest, "/?"); k >= 0 {
		hostport, tail = rest[:k], rest[k:]
	}
	hp := splitHostPort(hostport, scheme)
	if hp != nil && !strings.Contains(hostport, ":") {
		if up, known := a.s.sn.ups[hostport]; known {
			hp = up
		} else if !strings.Contains(hostport, ".") && net.ParseIP(hostport) == nil {
			hp = nil
		}
	}
	if !a.take(d, len(d.Args) == 1 && found && (scheme == "http" || scheme == "https") && hp != nil && !strings.Contains(raw, "$")) {
		return
	}
	a.loc.Upstream = &Upstream{Scheme: scheme, Host: hp.host, Port: hp.port}
	if used != nil {
		a.s.sn.disp[used.node] = "ignored"
	}
	if tail != "" {
		a.s.unm(d) // the URI part replaces the matched prefix; v1 cannot carry it
	}
}

// finish turns the collected proxy_set_header lines into flags and
// headers_set and records `set` lines no proxy_pass used.
func (a *locAcc) finish() {
	by := map[string]*hdr{}
	for i := range a.hdrs {
		h := &a.hdrs[i]
		if k := strings.ToLower(h.name); by[k] == nil {
			by[k] = h
		} else {
			a.s.unm(h.node)
		}
	}
	taken := map[*Block]bool{}
	all := true
	for k, v := range stdHeaders {
		all = all && by[k] != nil && by[k].val == v
	}
	for k := range stdHeaders {
		if all {
			taken[by[k].node] = true
		}
	}
	a.loc.ForwardedHeaders = all
	if u, c := by["upgrade"], by["connection"]; u != nil && c != nil && u.val == "$http_upgrade" && strings.EqualFold(c.val, "upgrade") {
		a.loc.WebSocket = true
		taken[u.node], taken[c.node] = true, true
	}
	for i := range a.hdrs {
		h := &a.hdrs[i]
		switch {
		case by[strings.ToLower(h.name)] != h:
		case taken[h.node]:
			a.s.sn.disp[h.node] = "mapped"
		case !strings.Contains(h.val, "$"):
			a.loc.HeadersSet[h.name] = h.val
			a.s.sn.disp[h.node] = "mapped"
		default:
			a.s.unm(h.node)
		}
	}
	for _, v := range a.local {
		if a.s.sn.disp[v.node] == "" {
			a.s.unm(v.node)
		}
	}
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
	if code < 301 || code > 308 || !(strings.HasPrefix(to, "https://") || strings.HasPrefix(to, "http://") || strings.HasPrefix(to, "/")) {
		return nil, false, false
	}
	ret.To = &to
	return ret, true, strings.Contains(to, "$") && !isHTTPSRedirect(ret)
}
