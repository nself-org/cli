package routemodel

// snippet_guard.go — directives that limit what a scope's mapping may claim.
//
// Purpose: find, in a server or location block, what makes a `set` variable
// or `deny all` unsafe to map: rewrite-phase flow, other variable writers and
// access rules that let requests through.
// Inputs: one block (server or location) or the top level of a file.
// Outputs: guard; the location's guard joins the server's.
// Constraints: conservative. A false hit only records a directive the model
// could have mapped; a miss would map a wrong value.

import "strings"

// guard is what one scope holds.
//   - flow: an if, rewrite, break or return, which can skip a later `set`
//     or set a variable conditionally.
//   - writes: variables another directive writes, by lower-case name: the
//     first argument of any directive but `set` and *_pass
//     (auth_request_set, set_by_lua ...).
//   - allow: an allow rule; with it `deny all` no longer denies everyone.
//     Only a location's own allow counts (nginx inherits access rules
//     all-or-none and the location has a deny).
//   - satisfyAny: `satisfy` other than all (any auth passing lets the request in).
type guard struct {
	flow, allow, satisfyAny bool
	writes                  map[string]bool
}

func scanGuard(b *Block) guard {
	g := guard{writes: map[string]bool{}}
	for _, d := range b.Children {
		switch d.Name {
		case "if", "rewrite", "break", "return":
			g.flow = true
		case "allow":
			g.allow = true
		case "satisfy":
			g.satisfyAny = g.satisfyAny || d.Arg(0) != "all"
		case "set":
		default: // *_pass only reads its target
			if strings.HasSuffix(d.Name, "_pass") {
				continue
			}
			if v, ok := strings.CutPrefix(d.Arg(0), "$"); ok {
				g.writes[strings.ToLower(v)] = true
			}
		}
	}
	return g
}

// join is the guard of a location l inside the server scope g.
func (g guard) join(l guard) guard {
	out := guard{flow: g.flow || l.flow, allow: l.allow, satisfyAny: g.satisfyAny || l.satisfyAny, writes: map[string]bool{}}
	for _, w := range []map[string]bool{g.writes, l.writes} {
		for k := range w {
			out.writes[k] = true
		}
	}
	return out
}

// globalVars names every variable an argument mentions outside any server
// block (map, geo, split_clients, js_set, perl_set ...): any of them may be
// written there.
func globalVars(tree []*Block) map[string]bool {
	out := map[string]bool{}
	for _, b := range tree {
		if b.Name == "server" {
			continue
		}
		for _, a := range unquoteAll(b.Args) {
			if v, ok := strings.CutPrefix(a, "$"); ok {
				out[strings.ToLower(v)] = true
			}
		}
	}
	return out
}
