package routemodel

// qualify.go — container-name upstreams for a shared host proxy.
//
// Purpose: EPIC ADOPT D2 (P7-ADOPT-03). Once one nginx reaches two projects, a
// bare upstream such as `hasura:8080` is ambiguous (every project has one). The
// shared-proxy build rewrites the upstream host of this project's own services
// to their container names and prefixes a fronted project's site files with
// its project name, so two projects' `hasura.conf` never meet.
// Inputs: a Model from Build; a host -> container-name mapping.
// Outputs: the Model changed in place; QualifyText returns changed conf text.
// Constraints: only core, optional and custom_service routes are rewritten (the
// services this project's compose set defines). Hand-managed and internal
// routes keep their upstreams verbatim (the caller only checks them); plugin
// snippet text is rewritten by QualifyText, host by host, and only for hosts in
// the mapping. A host already qualified, or not in the mapping, is untouched,
// so every function here is idempotent.

import (
	"regexp"
	"strings"
)

// QualifyUpstreams rewrites the upstream host of every generated service route
// whose host `container` maps (returning the new name and true).
func QualifyUpstreams(m *Model, container func(host string) (string, bool)) {
	for i := range m.Routes {
		r := &m.Routes[i]
		switch r.Source {
		case "core", "optional", "custom_service":
		default:
			continue
		}
		if r.RawUpstream != "" {
			r.RawUpstream = qualifyTarget(r.RawUpstream, container)
		}
		for j := range r.Locations {
			if up := r.Locations[j].Upstream; up != nil {
				if name, ok := container(up.Host); ok {
					up.Host = name
				}
			}
		}
	}
}

// qualifyTarget rewrites the host of a `[scheme://]host[:port][/path]` target.
func qualifyTarget(target string, container func(string) (string, bool)) string {
	rest, scheme := target, ""
	if i := strings.Index(target, "://"); i >= 0 {
		scheme, rest = target[:i+3], target[i+3:]
	}
	host, tail := rest, ""
	if i := strings.IndexAny(rest, ":/?"); i >= 0 {
		host, tail = rest[:i], rest[i:]
	}
	if name, ok := container(host); ok {
		return scheme + name + tail
	}
	return target
}

// PrefixSiteFiles prefixes the site file name of every route with
// `<project>.` (nginx/sites/hasura.conf -> nginx/sites/b.hasura.conf). A file
// that already carries the prefix is left alone.
func PrefixSiteFiles(m *Model, project string) {
	const dir = "nginx/sites/"
	for i := range m.Routes {
		f := m.Routes[i].File
		if strings.HasPrefix(f, dir) && !strings.HasPrefix(f[len(dir):], project+".") {
			m.Routes[i].File = dir + project + "." + f[len(dir):]
		}
	}
}

var (
	// `://host` in proxy_pass, set $x or return targets; host stops at a port,
	// path, `;`, space or quote.
	schemeHostRE = regexp.MustCompile(`(://)([A-Za-z0-9_.-]+)`)
	// `server host[:port]` inside an upstream block (`server_name` and
	// `server {` do not match: the keyword must be followed by blank space and
	// a host character).
	upstreamServerRE = regexp.MustCompile(`(?m)(^\s*server\s+)([A-Za-z0-9_.-]+)`)
)

// QualifyText rewrites the upstream hosts of an nginx conf text: `://host` and
// the host of an upstream block's `server host:port;` line, when container maps
// the host.
func QualifyText(text string, container func(host string) (string, bool)) string {
	sub := func(re *regexp.Regexp) {
		text = re.ReplaceAllStringFunc(text, func(m string) string {
			g := re.FindStringSubmatch(m)
			if name, ok := container(g[2]); ok {
				return g[1] + name
			}
			return m
		})
	}
	sub(schemeHostRE)
	sub(upstreamServerRE)
	return text
}
