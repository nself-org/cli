package plugincheck

// gate.go — is a rendered plugin snippet closed to the public?
//
// Purpose: `nself build` serves a plugin's nginx/*.conf on the public nginx
// only when the plugin's manifest declares the route public (nginx.public) or
// the conf already refuses every caller but loopback (D-0777). This file
// recognises the second case, the gate the plugins repo checks in
// scripts/ci/tests/test_plugin_nginx_access_gate.py.
// Inputs: the rendered snippet text. Outputs: LoopbackGated.
// Constraints: conservative. A false "not gated" only holds a conf back; a
// false "gated" would publish an unauthenticated plugin API.

import "github.com/nself-org/cli/internal/nginx/routemodel"

// loopback is the only allow a gated server may carry: private ranges are not
// enough, because Docker's userland proxy and tunnel containers deliver
// internet traffic from private addresses.
var loopback = map[string]bool{"127.0.0.1": true, "::1": true}

// bypass lists directives that answer, redirect or re-route a request before
// or instead of the access phase, or that rewrite $remote_addr from a header
// (a spoofed X-Forwarded-For: 127.0.0.1 would pass the loopback allow).
var bypass = map[string]bool{"if": true, "rewrite": true, "break": true, "return": true,
	"include": true, "error_page": true, "access_by_lua": true, "access_by_lua_block": true,
	"access_by_lua_file": true, "set_real_ip_from": true, "real_ip_header": true, "real_ip_recursive": true}

// LoopbackGated reports whether every server block in rendered refuses all
// callers but the nginx container's loopback: a server-level `deny all;`,
// server-level allows of 127.0.0.1 or ::1 only, no allow or deny in a nested
// block (it would replace the inherited rules), no `satisfy` other than all
// and no bypass directive anywhere in the file. A file without a server block
// is not gated.
func LoopbackGated(rendered string) bool {
	tree, err := routemodel.Parse(rendered)
	if err != nil || !noBypass(tree) {
		return false
	}
	servers := 0
	for _, b := range tree {
		if b.Name != "server" || !b.Block {
			continue
		}
		servers++
		if !serverGated(b) {
			return false
		}
	}
	return servers > 0
}

func serverGated(server *routemodel.Block) bool {
	denyAll := false
	for _, d := range server.Children {
		if d.Block {
			if hasAccessRule(d.Children) {
				return false
			}
			continue
		}
		switch d.Name {
		case "deny":
			if len(d.Args) != 1 || d.Arg(0) != "all" {
				return false
			}
			denyAll = true
		case "allow":
			if len(d.Args) != 1 || !loopback[d.Arg(0)] {
				return false
			}
		}
	}
	return denyAll
}

func hasAccessRule(blocks []*routemodel.Block) bool {
	for _, b := range blocks {
		if b.Name == "allow" || b.Name == "deny" || hasAccessRule(b.Children) {
			return true
		}
	}
	return false
}

func noBypass(blocks []*routemodel.Block) bool {
	for _, b := range blocks {
		if bypass[b.Name] || b.Name == "satisfy" && b.Arg(0) != "all" {
			return false
		}
		if !noBypass(b.Children) {
			return false
		}
	}
	return true
}
