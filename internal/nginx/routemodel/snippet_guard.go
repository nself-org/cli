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
//   - flow: an if, rewrite, break, return or include, which can skip a later
//     `set`, set a variable conditionally or hide either.
//   - writes: variables another directive writes, by lower-case name: the
//     first argument of any directive but `set` and *_pass
//     (auth_request_set, set_by_lua ...).
//   - allow: an allow rule, also inside if or limit_except; with it `deny
//     all` no longer denies everyone. Only a location's own allow counts
//     (nginx inherits access rules all-or-none and the location has a deny).
//   - satisfyAny: `satisfy` other than all (any auth passing lets the
//     request in); inherited from the top of the file and the server.
type guard struct {
	flow, allow, satisfyAny bool
	writes                  map[string]bool
}

func scanGuard(b *Block) guard {
	g := guard{writes: map[string]bool{}, allow: hasAllow(b)}
	for _, d := range b.Children {
		switch d.Name {
		case "if", "rewrite", "break", "return", "include":
			g.flow = true
		case "satisfy":
			g.satisfyAny = g.satisfyAny || d.Arg(0) != "all"
		case "set", "allow":
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

// hasAllow reports an allow in b or in a block under it other than a nested
// location (if, limit_except).
func hasAllow(b *Block) bool {
	for _, d := range b.Children {
		if d.Name == "allow" || d.Block && d.Name != "location" && hasAllow(d) {
			return true
		}
	}
	return false
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

// globalGuard is the guard of the directives outside any server block: every
// variable an argument names (map, geo, split_clients, js_set, perl_set ...
// may write it) and an inherited `satisfy`.
func globalGuard(tree []*Block) guard {
	g := guard{writes: map[string]bool{}}
	for _, b := range tree {
		if b.Name == "server" {
			continue
		}
		g.satisfyAny = g.satisfyAny || b.Name == "satisfy" && b.Arg(0) != "all"
		for _, a := range unquoteAll(b.Args) {
			if v, ok := strings.CutPrefix(a, "$"); ok {
				g.writes[strings.ToLower(v)] = true
			}
		}
	}
	return g
}

// locationSets names every variable a location under b sets. An internal
// redirect (rewrite ... last) carries that value into a sibling location
// without re-running the server's set, so the server's value is not certain.
func locationSets(b *Block, in bool, out map[string]bool) map[string]bool {
	for _, d := range b.Children {
		if in && d.Name == "set" {
			out[strings.ToLower(strings.TrimPrefix(d.Arg(0), "$"))] = true
		}
		if d.Block {
			locationSets(d, in || d.Name == "location", out)
		}
	}
	return out
}
