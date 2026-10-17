package routemodel

// snippet_limit.go — limit_except blocks as a route's method list.
//
// Purpose: map `limit_except M... { deny all; }` to the allowed methods.
// Inputs: one limit_except block of a location.
// Outputs: loc.Methods, or the block recorded as unmodelled.
// Constraints: only a closed scope maps (see locAcc.take and guard.closed).

// limitExcept maps `limit_except M... { deny all; }` to methods when the
// scope is closed (satisfy any or an allow would let other methods in).
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
	if a.take(d, ok && a.g.closed()) {
		a.loc.Methods = ms
	}
}
