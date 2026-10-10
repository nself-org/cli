package routemodel

// snippet.go — plugin nginx snippets and hand-managed conf.d files as routes.
//
// Purpose: read one nginx file into model routes, one per `server { }` block,
// with the closed directive vocabulary of EPIC DEPL D2 (snippet_map.go).
// Inputs: source ("plugin" or "hand_managed"), owner (plugin slug), file (the
// name used in route ids) and the file text.
// Outputs: Snippet{Routes, Global}. Each directive is mapped to a field,
// judged to have no observable effect, or copied verbatim into the route's
// `unmodelled` (inside a server block) or Global (outside one).
// Constraints: nothing is evaluated (no includes, maps or `if`). Whatever no
// handler claims is swept into `unmodelled`, so a directive is never dropped.

import (
	"fmt"
	"regexp"
	"strings"
)

// maxSnippetBytes bounds the text ParseSnippet accepts (snippets are small).
const maxSnippetBytes = 1 << 20

var (
	fqdnRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)
	certRE = regexp.MustCompile(`^/etc/nginx/ssl/([^/]+)/(fullchain|privkey)\.pem$`)
)

type hostPort struct {
	host string
	port int
}

// Snippet is the result of reading one nginx file.
type Snippet struct {
	Routes  []Route  // one per server block, in source order (Marshal sorts names and locations)
	Global  []string // verbatim directives outside any server block
	Rescued int      // nodes nothing claimed, recorded by the sweep (0 when the table covers the input)

	disp map[*Block]string    // mapped, ignored, unmodelled or container
	tree []*Block             // the parsed file (accounting checks walk it)
	ups  map[string]*hostPort // upstream{} blocks of this file; nil = not reducible to one host:port
	out  map[string]bool      // upstream names defined in other files of the same scan
}

// FromSnippet returns the routes of one nginx file. Directives outside any
// server block are added to every route's `unmodelled`; with no route to
// carry them it is an error (ParseSnippet returns them in Global).
func FromSnippet(source, owner, file, text string) ([]Route, error) {
	sn, err := ParseSnippet(source, owner, file, text)
	if err != nil {
		return nil, err
	}
	if len(sn.Global) > 0 && len(sn.Routes) == 0 {
		return nil, fmt.Errorf("%s: directives outside any server block and no route to carry them", file)
	}
	for i := range sn.Routes {
		sn.Routes[i].Unmodelled = append(sn.Routes[i].Unmodelled, sn.Global...)
	}
	return sn.Routes, nil
}

// ParseSnippet reads text with no knowledge of other files.
func ParseSnippet(source, owner, file, text string) (*Snippet, error) {
	return ParseSnippetIn(source, owner, file, text, nil)
}

// ParseSnippetIn reads text. upstreams are the names of the upstream{} blocks
// defined in the other files of the scan: nginx upstreams are global, so a
// portless proxy_pass to such a name is not a DNS host and is recorded. It
// fails on an unknown source, oversized text and structurally broken text.
func ParseSnippetIn(source, owner, file, text string, upstreams []string) (*Snippet, error) {
	if source != "plugin" && source != "hand_managed" {
		return nil, fmt.Errorf("unknown snippet source %q", source)
	}
	if len(text) > maxSnippetBytes {
		return nil, fmt.Errorf("%s: larger than %d bytes", file, maxSnippetBytes)
	}
	tree, err := Parse(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	sn := &Snippet{disp: map[*Block]string{}, tree: tree, ups: map[string]*hostPort{}, out: map[string]bool{}, Routes: []Route{}, Global: []string{}}
	for _, u := range upstreams {
		sn.out[strings.ToLower(u)] = true
	}
	for _, b := range tree {
		if b.Name == "upstream" && b.Block {
			sn.upstream(b)
		}
	}
	id, path := "hand:"+file, "nginx/conf.d/"+file
	if strings.Contains(file, "/") {
		path = "nginx/" + file
	}
	if source == "plugin" {
		id, path = "plugin:"+owner+"/"+file, "nginx/sites/"+owner+"-"+file
	}
	for _, b := range tree {
		switch {
		case sn.disp[b] != "":
		case b.Name == "server" && b.Block && len(b.Args) == 0:
			sn.Routes = append(sn.Routes, sn.server(b, fmt.Sprintf("%s#%d", id, len(sn.Routes)+1), source, owner, path))
		default:
			sn.global(b)
		}
	}
	return sn, nil
}

func (sn *Snippet) global(b *Block) {
	sn.disp[b] = "unmodelled"
	sn.Global = append(sn.Global, b.Render())
}

// srv is the working state of one server block.
type srv struct {
	sn     *Snippet
	r      *Route
	vars   map[string]*setVar
	inh    []*Block // server-level directives that locations inherit
	rec    map[*Block]bool
	protos []string
	cipher *string
	cert   string
	tls    bool
}

type setVar struct {
	val  string
	node *Block
}

// unm records d verbatim in the route's `unmodelled`, once.
func (s *srv) unm(d *Block) {
	s.sn.disp[d] = "unmodelled"
	if !s.rec[d] {
		s.rec[d] = true
		s.r.Unmodelled = append(s.r.Unmodelled, d.Render())
	}
}

// take marks d mapped when ok, else records it.
func (s *srv) take(d *Block, ok bool) bool {
	if !ok {
		s.unm(d)
		return false
	}
	s.sn.disp[d] = "mapped"
	return true
}

// server maps one server block to a route.
func (sn *Snippet) server(b *Block, id, source, owner, file string) Route {
	r := Route{ID: id, Source: source, File: file, ServerNames: []string{}, SecurityHeaders: map[string]string{},
		BlockedPaths: []BlockedPath{}, Locations: []Location{}, Unmodelled: []string{}}
	if source == "plugin" {
		r.Owner = &owner
	}
	s := &srv{sn: sn, r: &r, vars: map[string]*setVar{}, rec: map[*Block]bool{}, protos: []string{}}
	sn.disp[b] = "container"
	var locs []*Block
	var ret *Block // the first server-level return
	listens := 0
	for _, d := range b.Children {
		switch d.Name {
		case "listen":
			listens++
			s.listen(d)
		case "server_name":
			s.serverName(d)
		case "ssl_certificate", "ssl_certificate_key":
			s.certPath(d)
		case "ssl_protocols":
			s.tls = true
			s.protos = append(s.protos, unquoteAll(d.Args)...)
			s.take(d, len(d.Args) > 0)
		case "ssl_ciphers":
			s.tls = true
			if s.take(d, len(d.Args) == 1 && s.cipher == nil) {
				c := d.Arg(0)
				s.cipher = &c
			}
		case "http2", "resolver", "proxy_http_version", "proxy_buffer_size", "proxy_buffers", "proxy_busy_buffers_size":
			sn.disp[d] = "ignored"
		case "set":
			name := strings.TrimPrefix(d.Arg(0), "$")
			if s.take(d, len(d.Args) == 2 && strings.HasPrefix(d.Arg(0), "$") && s.vars[name] == nil) {
				s.vars[name] = &setVar{d.Arg(1), d}
				sn.disp[d] = "" // claimed only when a proxy_pass uses it
			}
		case "return":
			if ret == nil {
				ret = d
			} else {
				s.unm(d)
			}
		case "location":
			locs = append(locs, d)
		case "proxy_connect_timeout", "proxy_read_timeout", "proxy_send_timeout", "client_max_body_size", "access_log", "proxy_set_header":
			s.inh = append(s.inh, d)
		default:
			s.unm(d)
		}
	}
	if listens == 0 {
		r.Listen.HTTP = true // nginx listens on *:80 when a server has no listen
	}
	if r.Listen.HTTPS || s.tls {
		r.TLS = &RouteTLS{SSLDir: s.cert, Protocols: s.protos, Ciphers: s.cipher}
	}
	for _, l := range locs {
		s.location(l)
	}
	if ret != nil {
		s.serverReturn(ret, len(locs))
	}
	sn.sweep(b, s) // also records unused `set` lines and inherited directives no location took
	return r
}

// sweep records every node under b that no handler claimed.
func (sn *Snippet) sweep(b *Block, s *srv) {
	for _, c := range b.Children {
		switch sn.disp[c] {
		case "":
			sn.Rescued++
			s.unm(c)
		case "container":
			sn.sweep(c, s)
		}
	}
}

// listen maps `listen 80;` and `listen 443 ssl [http2];` (also [::]:port and
// 0.0.0.0:port) to the listen flags; any other address, port or parameter is
// recorded.
func (s *srv) listen(d *Block) {
	a := unquoteAll(d.Args)
	addr := ""
	if len(a) > 0 {
		addr = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(a[0], "[::]:"), "0.0.0.0:"), "*:")
	}
	ssl, ok := false, len(a) > 0
	for _, p := range a[min(1, len(a)):] {
		ssl = ssl || p == "ssl"
		ok = ok && (p == "ssl" || p == "http2" || p == "ipv6only=on" || p == "ipv6only=off")
	}
	if s.take(d, ok && ((addr == "80" && !ssl) || (addr == "443" && ssl))) {
		s.r.Listen.HTTP = s.r.Listen.HTTP || addr == "80"
		s.r.Listen.HTTPS = s.r.Listen.HTTPS || addr == "443"
	}
}

// serverName adds the FQDNs; a wildcard, regex, `_` or variable name records
// the whole directive.
func (s *srv) serverName(d *Block) {
	ok := len(d.Args) > 0
	for _, n := range unquoteAll(d.Args) {
		if fqdnRE.MatchString(n) {
			s.r.ServerNames = appendUnique(s.r.ServerNames, strings.ToLower(n))
		} else {
			ok = false
		}
	}
	s.take(d, ok)
}

// certPath maps /etc/nginx/ssl/<dir>/fullchain.pem and privkey.pem to the
// route's ssl_dir; another path, file name or a second directory is recorded.
func (s *srv) certPath(d *Block) {
	s.tls = true
	m := certRE.FindStringSubmatch(d.Arg(0))
	want := map[string]string{"ssl_certificate": "fullchain", "ssl_certificate_key": "privkey"}[d.Name]
	if s.take(d, len(d.Args) == 1 && m != nil && m[2] == want && (s.cert == "" || s.cert == m[1])) {
		s.cert = m[1]
	}
}
