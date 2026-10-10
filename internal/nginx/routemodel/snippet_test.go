package routemodel

// snippet_test.go — corpus, mapping, accounting and tokenizer tests for the
// plugin snippet and hand-managed conf.d reader (P7-DEPL-22).
//
// The corpus is the 55 free-plugin nginx snippets at the commit named in
// testdata/snippets/SOURCE. Goldens are regenerated with
// `go test ./internal/nginx/routemodel/ -run TestSnippetCorpus -update`.

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite snippet goldens")

// renderVars stands in for the build's template variables.
var renderVars = strings.NewReplacer("${BASE_DOMAIN}", "example.test", "${DOMAIN}", "example.test",
	"${NSELF_DOMAIN}", "example.test", "${SSL_DIR}", "example-test", "${GRAFANA_ROUTE}", "grafana",
	"${PROMETHEUS_ROUTE}", "prometheus", "${ALERTMANAGER_ROUTE}", "alertmanager")

type corpusFile struct{ slug, name, path string }

func corpus(t *testing.T) []corpusFile {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "snippets", "*", "*.conf"))
	if err != nil || len(paths) != 55 {
		t.Fatalf("corpus has %d files, want 55 (err=%v)", len(paths), err)
	}
	var out []corpusFile
	for _, p := range paths {
		out = append(out, corpusFile{filepath.Base(filepath.Dir(p)), filepath.Base(p), p})
	}
	return out
}

// corpusFiles returns the rendered text of every corpus file (fuzz seeds).
func corpusFiles() []string {
	paths, _ := filepath.Glob(filepath.Join("testdata", "snippets", "*", "*.conf"))
	var out []string
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			out = append(out, renderVars.Replace(string(b)))
		}
	}
	return out
}

func readCorpus(t *testing.T, c corpusFile) string {
	t.Helper()
	b, err := os.ReadFile(c.path)
	if err != nil {
		t.Fatal(err)
	}
	return renderVars.Replace(string(b))
}

// route parses text as a plugin snippet and returns the first route.
func route(t *testing.T, text string) Route {
	t.Helper()
	rs, err := FromSnippet("plugin", "p", "f.conf", text)
	if err != nil || len(rs) == 0 {
		t.Fatalf("FromSnippet: %v routes=%d\n%s", err, len(rs), text)
	}
	return rs[0]
}

// directiveName returns the first word of a recorded directive text.
func directiveName(s string) string { return strings.Fields(strings.TrimLeft(s, "{ "))[0] }

func TestSnippetCorpus(t *testing.T) {
	counts := map[string]int{}
	routes, locationOnly := 0, 0
	for _, c := range corpus(t) {
		text := readCorpus(t, c)
		sn, err := ParseSnippet("plugin", c.slug, c.name, text)
		if err != nil {
			t.Fatalf("%s/%s: %v", c.slug, c.name, err)
		}
		blocks := len(regexp.MustCompile(`(?m)^server\s*\{`).FindAllString(text, -1))
		if len(sn.Routes) != blocks || (blocks == 0) != (len(sn.Global) > 0 && strings.HasPrefix(sn.Global[len(sn.Global)-1], "location ")) {
			t.Errorf("%s/%s: %d routes, %d server blocks, global %q", c.slug, c.name, len(sn.Routes), blocks, sn.Global)
		}
		if blocks == 0 {
			locationOnly++
		}
		if sn.Rescued != 0 {
			t.Errorf("%s/%s: %d directives fell through the table", c.slug, c.name, sn.Rescued)
		}
		for _, r := range sn.Routes {
			routes++
			if !strings.HasPrefix(r.ID, "plugin:"+c.slug+"/"+c.name+"#") || len(r.ServerNames) == 0 || r.File != "nginx/sites/"+c.slug+"-"+c.name {
				t.Errorf("%s/%s: bad route identity %+v", c.slug, c.name, r)
			}
			seen := map[string]bool{}
			for _, u := range r.Unmodelled {
				if n := directiveName(u); !seen[n] {
					seen[n] = true
					counts[n]++
				}
			}
		}
		b := goldenJSON(t, sn)
		golden := strings.TrimSuffix(c.path, ".conf") + ".routes.json"
		if *update {
			if err := os.WriteFile(golden, b, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil || string(want) != string(b) {
			t.Errorf("%s/%s: routes differ from %s (run with -update after review)", c.slug, c.name, golden)
		}
	}
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t.Logf("corpus routes with unmodelled %-12s %d (of %d routes)", n, counts[n], routes)
	}
	t.Logf("%d routes; %d location-only files have no server block (their locations are in unmodelled_global)", routes, locationOnly)
}

// goldenJSON is the routes and unmodelled_global of sn in contract order.
func goldenJSON(t *testing.T, sn *Snippet) []byte {
	t.Helper()
	full, err := Marshal(&Model{Routes: sn.Routes, UnmodelledGlobal: sn.Global})
	if err != nil {
		t.Fatal(err)
	}
	var part struct {
		UnmodelledGlobal []string `json:"unmodelled_global"`
		Routes           []Route  `json:"routes"`
	}
	if err := json.Unmarshal(full, &part); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(part, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

// words counts the words in text without using the tokenizer: comments cut
// per line, then a split on whitespace and ; { }.
func words(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		out = append(out, strings.FieldsFunc(line, func(r rune) bool { return strings.ContainsRune(" \t\r;{}", r) })...)
	}
	return out
}

func flatten(bs []*Block, out *[]string) {
	for _, b := range bs {
		if b.Name != "" {
			*out = append(*out, b.Name)
		}
		*out = append(*out, b.Args...)
		flatten(b.Children, out)
	}
}

// accounted checks that every node was mapped, ignored or recorded, and that
// a recorded node's text really is in the output lists.
func accounted(t *testing.T, label string, sn *Snippet, bs []*Block, recorded string) {
	t.Helper()
	for _, b := range bs {
		switch sn.disp[b] {
		case "":
			t.Errorf("%s: %s has no disposition", label, b.Render())
		case "container":
			accounted(t, label, sn, b.Children, recorded)
		case "unmodelled":
			if !strings.Contains(recorded, b.Render()) {
				t.Errorf("%s: %s is marked unmodelled but missing from the output", label, b.Render())
			}
		}
	}
}

func TestSnippetNoLostToken(t *testing.T) {
	var inputs = map[string]string{
		"weird": `map $a $b { x y; }
limit_req_zone $binary_remote_addr zone=z:10m rate=1r/s;
upstream two { server a:1; server b:2; }
server { listen 443 ssl; server_name a.test; weird_directive 1 2 3; if ($x) { return 403; }
  location ~ ^/x { proxy_pass http://two; } location /y { proxy_pass http://two; unknown_thing on; auth_basic "xy"; } }`,
		"compact": `server{listen 80;server_name b.test;location /{proxy_pass http://b:80/z;proxy_pass http://c:1;}}`,
	}
	for _, c := range corpus(t) {
		inputs[c.slug+"/"+c.name] = readCorpus(t, c)
	}
	for label, text := range inputs {
		sn, err := ParseSnippet("hand_managed", "", "x.conf", text)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		var all []string
		for _, r := range sn.Routes {
			all = append(all, r.Unmodelled...)
		}
		accounted(t, label, sn, sn.tree, strings.Join(append(all, sn.Global...), "\n"))
		var got []string
		flatten(Tokenize(text), &got)
		if want := words(text); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: tokenizer lost or invented words: %d vs %d", label, len(got), len(want))
		}
	}
	// the sweep must be the only thing standing between an unclaimed node and a loss
	sn, _ := ParseSnippet("plugin", "p", "f.conf", `server { listen 80; server_name a.test; zzz 1; location / { yyy 2; } }`)
	if len(sn.Routes[0].Unmodelled) != 2 || sn.Rescued != 0 {
		t.Errorf("unknown directives not recorded by the table: %v rescued=%d", sn.Routes[0].Unmodelled, sn.Rescued)
	}
}

func TestSnippetMapping(t *testing.T) {
	const head = "server { listen 443 ssl http2; server_name a.test; ssl_certificate /etc/nginx/ssl/d/fullchain.pem; ssl_certificate_key /etc/nginx/ssl/d/privkey.pem; "
	loc := func(body string) Location {
		r := route(t, head+"location / { "+body+" } }")
		if len(r.Unmodelled) != 0 {
			t.Fatalf("%q recorded %v", body, r.Unmodelled)
		}
		return r.Locations[0]
	}
	l := loc(`proxy_connect_timeout 5s; proxy_read_timeout 2m; proxy_send_timeout 1h30m; client_max_body_size 2g;`)
	if *l.Timeouts.ConnectS != 5 || *l.Timeouts.ReadS != 120 || *l.Timeouts.SendS != 5400 || *l.MaxBodyBytes != 2<<30 {
		t.Errorf("timeouts/body: %+v %v", l.Timeouts, *l.MaxBodyBytes)
	}
	for in, want := range map[string]int64{"1k": 1024, "10M": 10 << 20, "7": 7} {
		if got := *loc("client_max_body_size " + in + ";").MaxBodyBytes; got != want {
			t.Errorf("client_max_body_size %s = %d, want %d", in, got, want)
		}
	}
	if !loc(`deny all;`).DenyAll || loc(`access_log off;`).AccessLog || !loc(`proxy_pass http://x.test:81;`).AccessLog {
		t.Error("deny/access_log")
	}
	if got := loc(`limit_except GET POST { deny all; }`).Methods; !reflect.DeepEqual(got, []string{"GET", "POST"}) {
		t.Errorf("methods = %v", got)
	}
	std := `proxy_set_header Host $host; proxy_set_header X-Real-IP $remote_addr; proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for; proxy_set_header X-Forwarded-Proto $scheme; `
	if l := loc(std + `proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade"; proxy_set_header X-A b; proxy_set_header Connection2 "";`); !l.ForwardedHeaders || !l.WebSocket || !reflect.DeepEqual(l.HeadersSet, map[string]string{"X-A": "b", "Connection2": ""}) {
		t.Errorf("headers: %+v", l)
	}
	if l := loc(`proxy_pass http://x.test:81; proxy_http_version 1.1; proxy_buffers 8 4k; proxy_buffer_size 4k; proxy_busy_buffers_size 8k; resolver 127.0.0.11;`); *l.Upstream != (Upstream{"http", "x.test", 81}) {
		t.Errorf("upstream: %+v", l.Upstream)
	}
	// http to https redirect
	r := route(t, `server { listen 80; server_name a.test; return 301 https://$host$request_uri; }`)
	if !r.HTTPToHTTPSRedirect || !r.Listen.HTTP || r.Listen.HTTPS || len(r.Unmodelled) != 0 || len(r.Locations) != 1 ||
		r.Locations[0].Return == nil || r.Locations[0].Return.Status != 301 || *r.Locations[0].Return.To != "https://$host$request_uri" {
		t.Errorf("redirect: %+v", r)
	}
	// tls and listen
	r = route(t, head+`ssl_protocols TLSv1.2 TLSv1.3; ssl_ciphers HIGH:!aNULL; }`)
	if !r.Listen.HTTPS || r.TLS == nil || r.TLS.SSLDir != "d" || !reflect.DeepEqual(r.TLS.Protocols, []string{"TLSv1.2", "TLSv1.3"}) || *r.TLS.Ciphers != "HIGH:!aNULL" || len(r.Unmodelled) != 0 {
		t.Errorf("tls: %+v %+v", r, r.TLS)
	}
	// upstream block, set variable (location and server scope), upstream URI part
	sn, err := ParseSnippet("plugin", "p", "f.conf", `upstream u { server 10.0.0.1:9000; keepalive 8; } server { listen 80; server_name a.test; resolver 1.1.1.1; set $v http://svc:3000;
	  location /a { proxy_pass http://u; } location /b { proxy_pass $v; } location /c { set $w 127.0.0.1:99; proxy_pass http://$w/; } }`)
	if err != nil || len(sn.Global) != 1 {
		t.Fatalf("%v %q", err, sn.Global)
	}
	r = sn.Routes[0]
	got := map[string]Upstream{}
	for _, l := range r.Locations {
		got[l.Path] = *l.Upstream
	}
	want := map[string]Upstream{"/a": {"http", "10.0.0.1", 9000}, "/b": {"http", "svc", 3000}, "/c": {"http", "127.0.0.1", 99}}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(r.Unmodelled, []string{"proxy_pass http://$w/;"}) {
		t.Errorf("upstreams: %v unmodelled=%v", got, r.Unmodelled)
	}
}

// TestSnippetUnmodelled is the negative side of the mapping table: each input
// must be recorded verbatim, never mapped and never dropped.
func TestSnippetUnmodelled(t *testing.T) {
	const head = "server { listen 80; server_name a.test; "
	for in, want := range map[string]string{
		"location / { proxy_pass http://x.test/v1/; }":                 "proxy_pass http://x.test/v1/;",
		"location / { proxy_pass http://backend; }":                    "proxy_pass http://backend;",
		"location / { proxy_pass $nope; }":                             "proxy_pass $nope;",
		"location / { client_max_body_size 0; }":                       "client_max_body_size 0;",
		"location / { proxy_read_timeout 500ms; }":                     "proxy_read_timeout 500ms;",
		"location / { deny 10.0.0.0/8; }":                              "deny 10.0.0.0/8;",
		"location / { access_log /var/log/x; }":                        "access_log /var/log/x;",
		"location / { proxy_set_header Host $host; }":                  "proxy_set_header Host $host;",
		"location / { proxy_set_header X-Id $request_id; }":            "proxy_set_header X-Id $request_id;",
		"location / { return 200 \"ok\"; }":                            `return 200 "ok";`,
		"location / { limit_except GET { allow 1.1.1.1; deny all; } }": "limit_except GET { allow 1.1.1.1; deny all; }",
		"location / { limit_except GET { deny 1.2.3.4; } }":            "limit_except GET { deny 1.2.3.4; }",
		"location ~* \\.log$ { deny all; return 404; }":                "location ~* \\.log$ { deny all; return 404; }",
		"location / { if ($a) { return 403; } }":                       "if ($a) { return 403; }",
		"location ^~ /x { }":                                           "location ^~ /x { }",
		"proxy_set_header Host $host; location / { }":                  "proxy_set_header Host $host;",
		"listen 127.0.0.1:81;":                                         "listen 127.0.0.1:81;",
		"listen 80 default_server;":                                    "listen 80 default_server;",
		"server_name *.test;":                                          "server_name *.test;",
		"location / { } location / { }":                                "location / { }",
		"ssl_certificate /etc/other/x.pem;":                            "ssl_certificate /etc/other/x.pem;",
		"set $unused 1;":                                               "set $unused 1;",
		"return 200 ok; location / { }":                                "return 200 ok;",
	} {
		r := route(t, head+in+" }")
		if !reflect.DeepEqual(r.Unmodelled[len(r.Unmodelled)-1:], []string{want}) && !contains(r.Unmodelled, want) {
			t.Errorf("%q: unmodelled = %v, want %q", in, r.Unmodelled, want)
		}
	}
}

func contains(l []string, s string) bool {
	for _, e := range l {
		if e == s {
			return true
		}
	}
	return false
}

func TestUnmodelledGlobal(t *testing.T) {
	text := "map $http_upgrade $connection_upgrade { default upgrade; '' close; }\nlimit_req_zone $binary_remote_addr zone=z:10m rate=1r/s;\n" +
		"upstream lb { server a:1; server b:2; }\nserver { listen 80; server_name a.test; location / { proxy_pass http://lb; } }\nlimit_conn_zone $x zone=c:1m;"
	sn, err := ParseSnippet("hand_managed", "", "x.conf", text)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"map $http_upgrade $connection_upgrade { default upgrade; '' close; }", "limit_req_zone $binary_remote_addr zone=z:10m rate=1r/s;",
		"upstream lb { server a:1; server b:2; }", "limit_conn_zone $x zone=c:1m;"}
	if !reflect.DeepEqual(sn.Global, want) {
		t.Errorf("Global = %q", sn.Global)
	}
	if r := sn.Routes[0]; r.ID != "hand:x.conf#1" || r.Source != "hand_managed" || r.Owner != nil || r.File != "nginx/conf.d/x.conf" ||
		r.Locations[0].Upstream != nil || !reflect.DeepEqual(r.Unmodelled, []string{"proxy_pass http://lb;"}) {
		t.Errorf("route = %+v", r)
	}
	if _, err := FromSnippet("hand_managed", "", "y.conf", "limit_req_zone $a zone=z:1m rate=1r/s;"); err == nil {
		t.Error("global directive without a route was accepted by FromSnippet")
	}
	if rs, _ := FromSnippet("hand_managed", "", "x.conf", text); !contains(rs[0].Unmodelled, want[1]) {
		t.Error("FromSnippet did not fold the global directives into the route")
	}
	for _, bad := range []string{"server { listen 80;", "server { listen 80 }", "a b", `server { x "y; }`, "}"} {
		if _, err := ParseSnippet("plugin", "p", "f.conf", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := ParseSnippet("other", "", "f", ""); err == nil {
		t.Error("unknown source accepted")
	}
	if _, err := ParseSnippet("plugin", "p", "f", strings.Repeat("#", maxSnippetBytes+1)); err == nil {
		t.Error("oversized text accepted")
	}
}

func TestTokenizeAdapter(t *testing.T) {
	cases := map[string]string{
		"a  b\tc ;":                           "a b c;",
		"server{listen 80;}":                  "server { listen 80; }",
		"x \"a;b{c}\" 'd#e'; # gone\ny z;":    `x "a;b{c}" 'd#e'; y z;`,
		"proxy_pass http://${up}/x; a#b c;":   "proxy_pass http://${up}/x; a#b c;",
		"{ a; }":                              "{ a; }",
		"location ~ ^/(a|b)$ { return 404; }": "location ~ ^/(a|b)$ { return 404; }",
		"a b\nc d;":                           "a b c d;",
	}
	for in, want := range cases {
		var out []string
		for _, b := range Tokenize(in) {
			out = append(out, b.Render())
		}
		if got := strings.Join(out, " "); got != want {
			t.Errorf("Tokenize(%q) = %q, want %q", in, got, want)
		}
	}
	compact := `server{listen 443 ssl;server_name a.test;location /{proxy_pass http://u:1;}}`
	multi := "server {\n  listen 443 ssl;\n  server_name a.test; # c } {\n  location / {\n    proxy_pass http://u:1;\n  }\n}\n"
	if a, b := Tokenize(compact)[0].Render(), Tokenize(multi)[0].Render(); a != b {
		t.Errorf("compact and multi-line forms differ:\n%s\n%s", a, b)
	}
	b := Tokenize(`x "a\"b" 'c';`)[0]
	if b.Arg(0) != `a"b` || b.Arg(1) != "c" || b.Arg(2) != "" || Unquote(`"x`) != `"x` {
		t.Errorf("Arg/Unquote: %q %q", b.Arg(0), b.Arg(1))
	}
	if _, err := Parse("a { b; }"); err != nil {
		t.Error(err)
	}
	enc, _ := json.Marshal(Tokenize("a b;")[0].Args)
	if string(enc) != `["b"]` {
		t.Error(string(enc))
	}
}

func TestHandManagedRoutes(t *testing.T) {
	text := "server { listen 80; server_name a.example.test; location / { proxy_pass http://a:80; } }\n" +
		"server { listen 443 ssl; server_name b.example.test; ssl_certificate /etc/nginx/ssl/d/fullchain.pem; ssl_certificate_key /etc/nginx/ssl/d/privkey.pem; location /x { proxy_pass https://b.example.test; } }\n"
	rs, err := FromSnippet("hand_managed", "", "custom.conf", text)
	if err != nil || len(rs) != 2 {
		t.Fatalf("routes=%d err=%v", len(rs), err)
	}
	if rs[0].ID != "hand:custom.conf#1" || rs[1].ID != "hand:custom.conf#2" || rs[0].Source != "hand_managed" || rs[0].Owner != nil ||
		rs[0].File != "nginx/conf.d/custom.conf" || !rs[0].Listen.HTTP || !rs[1].Listen.HTTPS || *rs[1].Locations[0].Upstream != (Upstream{"https", "b.example.test", 443}) {
		t.Errorf("hand routes wrong: %+v", rs)
	}
	rs, _ = FromSnippet("hand_managed", "", "conf.d-prod/x.conf", "server { listen 80; server_name c.test; }")
	if rs[0].ID != "hand:conf.d-prod/x.conf#1" || rs[0].File != "nginx/conf.d-prod/x.conf" {
		t.Errorf("env dir route wrong: %+v", rs[0])
	}
}

// TestSnippetNginxLexing: words are read as nginx reads them (the reviewer's
// hostile inputs, proven against nginx 1.27.5): a quote opens only at the start
// of a word, a backslash escapes outside quotes, a } inside a word is literal.
func TestSnippetNginxLexing(t *testing.T) {
	r := route(t, "server { listen 80; server_name a.test; location / { proxy_pass http://127.0.0.1:3000;\n proxy_set_header X-A a\"; deny all; # \"\n } }")
	l := r.Locations[0]
	if !l.DenyAll || l.HeadersSet["X-A"] != `a"` || len(r.Unmodelled) != 0 {
		t.Errorf("mid-word quote swallowed a directive: deny=%v headers=%v unmodelled=%v", l.DenyAll, l.HeadersSet, r.Unmodelled)
	}
	r = route(t, "server { listen 80; server_name a.test; location / { proxy_pass http://127.0.0.1:3000;\n proxy_set_header X-A \\\"; return 418; # \"\n } }")
	if l := r.Locations[0]; l.Return == nil || l.Return.Status != 418 || l.HeadersSet["X-A"] != `"` {
		t.Errorf("escaped quote swallowed a directive: %+v", l)
	}
	for in, want := range map[string]string{`a}b;`: "a}b;", `a "b;c" d;`: `a "b;c" d;`, `a 'x"y';`: `a 'x"y';`, `a\;b c;`: `a\;b c;`, `a"b;`: `a"b;`} {
		if got := Tokenize(in)[0].Render(); got != want {
			t.Errorf("Tokenize(%q) = %q, want %q", in, got, want)
		}
	}
	if b := Tokenize(`a "b"c;`); len(b) == 0 {
		t.Error("no tree for glued quote")
	}
	if _, err := Parse(`a "b"c;`); err == nil {
		t.Error("text glued to a closing quote accepted")
	}
	if got := Unquote(`"a\"b\\c\td"`); got != "a\"b\\c\td" {
		t.Errorf("Unquote = %q", got)
	}
}

func TestSnippetUpstreamsAreGlobal(t *testing.T) {
	hand := "upstream api.internal { server 10.1.1.1:9000; keepalive 8; }\nupstream two { server a:1; server b:2; }"
	sn, err := ParseSnippet("hand_managed", "", "x.conf", hand)
	if err != nil || len(sn.Global) != 2 || sn.Global[0] != "upstream api.internal { server 10.1.1.1:9000; keepalive 8; }" {
		t.Fatalf("upstreams dropped or not verbatim: %v %q", err, sn.Global)
	}
	plug := "server { listen 80; server_name p.test; location / { proxy_pass http://api.internal; } }"
	if sn, _ = ParseSnippetIn("plugin", "p", "f.conf", plug, nil); sn.Routes[0].Locations[0].Upstream == nil || sn.Routes[0].Locations[0].Upstream.Port != 80 {
		t.Errorf("dotted name with no upstream of that name should be a host: %+v", sn.Routes[0].Locations[0])
	}
	sn, _ = ParseSnippetIn("plugin", "p", "f.conf", plug, UpstreamNames(hand))
	if r := sn.Routes[0]; r.Locations[0].Upstream != nil || !reflect.DeepEqual(r.Unmodelled, []string{"proxy_pass http://api.internal;"}) {
		t.Errorf("proxy_pass to an upstream of another file was resolved as a host: %+v", r)
	}
	if got := UpstreamNames(hand); !reflect.DeepEqual(got, []string{"api.internal", "two"}) {
		t.Errorf("UpstreamNames = %v", got)
	}
	own := "upstream u { server 10.0.0.1:9000; } server { listen 80; server_name p.test; location / { proxy_pass http://U; } }"
	sn, _ = ParseSnippet("plugin", "p", "f.conf", own)
	if sn.Routes[0].Locations[0].Upstream == nil || sn.Routes[0].Locations[0].Upstream.Port != 9000 || len(sn.Global) != 1 {
		t.Errorf("in-file upstream: %+v global=%q", sn.Routes[0].Locations[0], sn.Global)
	}
}

func TestSnippetDefaultListenAndNits(t *testing.T) {
	r := route(t, "server { server_name A.Example.COM; location / { proxy_pass http://x.test:3000; } }")
	if !r.Listen.HTTP || r.Listen.HTTPS || len(r.Unmodelled) != 0 || !reflect.DeepEqual(r.ServerNames, []string{"a.example.com"}) {
		t.Errorf("no listen: %+v", r)
	}
	if r = route(t, "server { listen 8080; server_name a.test; }"); r.Listen.HTTP || len(r.Unmodelled) != 1 {
		t.Errorf("listen 8080 must not become port 80: %+v", r)
	}
	for _, body := range []string{"return 403; proxy_pass http://x.test:1;", "proxy_pass http://x.test:1; return 403;"} {
		r = route(t, "server { listen 80; server_name a.test; location / { "+body+" } }")
		if !reflect.DeepEqual(r.Unmodelled, []string{"return 403;"}) {
			t.Errorf("%q: unmodelled = %v", body, r.Unmodelled)
		}
	}
	r = route(t, "server { listen 80; server_name a.test; location / { deny all; deny 1.2.3.4; access_log off; access_log /x; } }")
	if !r.Locations[0].DenyAll || r.Locations[0].AccessLog {
		t.Errorf("a later directive reset a mapped flag: %+v", r.Locations[0])
	}
	for in, want := range map[string]bool{"1m1s": true, "1s1m": false, "1m1m": false, "1h30m": true, "1s500ms": false, "5": true} {
		_, ok := parseSeconds(Tokenize("x " + in + ";")[0])
		if ok != want {
			t.Errorf("parseSeconds(%s) ok=%v want %v", in, ok, want)
		}
	}
	if _, ok := parseBodySize(Tokenize("x 17179869185g;")[0]); ok {
		t.Error("overflowing body size accepted")
	}
	if n, ok := parseBodySize(Tokenize("x 8g;")[0]); !ok || n != 8<<30 {
		t.Errorf("8g = %d %v", n, ok)
	}
}
