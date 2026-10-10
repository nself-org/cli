package routemodel

// snippet_upstream.go — upstream{} blocks in plugin snippets and conf.d files.
//
// Purpose: resolve `proxy_pass http://name` when name is an upstream with one
// server, and name every upstream a scan defines.
// Inputs: the top-level upstream{} blocks of one file.
// Outputs: Snippet.ups (name to host and port, nil when not reducible).
// Constraints: nginx upstreams are http-level and visible to every file, so
// each one is also recorded verbatim in Global (never dropped), and a
// proxy_pass name defined in another file is not resolved (see ParseSnippetIn).

import "strings"

// UpstreamNames returns the names of the top-level upstream{} blocks of text.
func UpstreamNames(text string) []string {
	var names []string
	for _, b := range Tokenize(text) {
		if b.Name == "upstream" && b.Block && len(b.Args) == 1 {
			names = append(names, strings.ToLower(b.Arg(0)))
		}
	}
	return names
}

// upstream reduces `upstream n { server host:port; keepalive N; }` to one
// host and port. Anything else (several servers, weights, balancing, a unix
// socket, a repeated name) leaves the name unresolvable, so a proxy_pass
// naming it is recorded too. The block itself stays unclaimed: ParseSnippet
// records every upstream in Global, in source order.
func (sn *Snippet) upstream(b *Block) {
	name := strings.ToLower(b.Arg(0))
	var hp *hostPort
	_, dup := sn.ups[name]
	servers, clean := 0, len(b.Args) == 1 && !dup
	for _, d := range b.Children {
		switch {
		case d.Name == "server" && len(d.Args) == 1 && !d.Block:
			servers++
			hp = splitHostPort(d.Arg(0), "http")
		case d.Name == "keepalive" && len(d.Args) == 1 && !d.Block && strings.Trim(d.Arg(0), "0123456789") == "" && d.Arg(0) != "":
		default:
			clean = false
		}
	}
	if !clean || servers != 1 {
		hp = nil
	}
	sn.ups[name] = hp
}
