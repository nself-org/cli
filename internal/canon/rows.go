package canon

// Row validation for contract:cli.canon-fragments v1: path shape, versions,
// required fields. Cross-fragment rules (uniqueness, resolution) live in
// domains.go (merge) and view.go (resolution against the v1.5 view).

import (
	"fmt"
	"strconv"
	"strings"
)

// validPath reports whether s is a command path of single-space-separated
// words without the leading "nself ".
func validPath(s string) bool {
	return s != "" && s == strings.Join(strings.Fields(s), " ") && !strings.HasPrefix(s, "nself ")
}

// parseVersion returns the numeric parts of a vX.Y.Z string.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return v, false
	}
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

// versionLess reports whether a < b for two valid versions.
func versionLess(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// validate checks every row of r; each problem is prefixed with where.
func (r Rows) validate(where string) []string {
	var p []string
	bad := func(list string, i int, from, msg string) {
		p = append(p, fmt.Sprintf("%s%s[%d] (%q): %s", where, list, i, from, msg))
	}
	pathOK := func(list string, i int, from, field, v string) {
		if !validPath(v) {
			bad(list, i, from, fmt.Sprintf("%s %q must be a command path without the leading \"nself \"", field, v))
		}
	}
	version := func(list string, i int, from, field, v string) {
		if _, ok := parseVersion(v); !ok {
			bad(list, i, from, fmt.Sprintf("%s %q must look like vX.Y.Z", field, v))
		}
	}
	for i, h := range r.Hubs {
		pathOK("hubs", i, h.Path, "path", h.Path)
		if h.Summary == "" {
			bad("hubs", i, h.Path, "summary is required")
		}
	}
	for i, b := range r.Builtins {
		pathOK("builtins", i, b, "name", b)
	}
	rows := []struct {
		list  string
		rows  []Row
		equiv bool
	}{{"moves", r.Moves, false}, {"shims", r.Shims, true}, {"retired_hubs", r.RetiredHubs, false}}
	for _, g := range rows {
		for i, x := range g.rows {
			pathOK(g.list, i, x.From, "from", x.From)
			pathOK(g.list, i, x.From, "to", x.To)
			if x.From == x.To {
				bad(g.list, i, x.From, "to must differ from from")
			}
			version(g.list, i, x.From, "since", x.Since)
			version(g.list, i, x.From, "removal_at", x.RemovalAt)
			s, ok1 := parseVersion(x.Since)
			e, ok2 := parseVersion(x.RemovalAt)
			if ok1 && ok2 && !versionLess(s, e) {
				bad(g.list, i, x.From, fmt.Sprintf("removal_at %s must be after since %s", x.RemovalAt, x.Since))
			}
			if g.equiv && x.Equivalence == "" {
				bad(g.list, i, x.From, "equivalence (the Go test proving the forward) is required on a shim")
			}
		}
	}
	for i, b := range r.Breakouts {
		pathOK("breakouts", i, b.From, "from", b.From)
		pathOK("breakouts", i, b.From, "to", b.To)
		if !pluginRe.MatchString(b.Plugin) {
			bad("breakouts", i, b.From, fmt.Sprintf("plugin %q must match ^[a-z][a-z0-9-]*$", b.Plugin))
		}
		version("breakouts", i, b.From, "since", b.Since)
	}
	for i, x := range r.Removed {
		pathOK("removed", i, x.From, "from", x.From)
		version("removed", i, x.From, "since", x.Since)
		if x.Message == "" {
			bad("removed", i, x.From, "message is required")
		}
	}
	return p
}
