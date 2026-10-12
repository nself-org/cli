package build

// shared_proxy_check.go — what a project puts on a shared proxy, and the
// cross-project collision checks (E503 names and certificates, E504 upstream
// hosts).
//
// Purpose: EPIC ADOPT D2 isolation. A name, certificate directory or bare
// upstream host that another project already holds must fail the build before
// anything is written, naming both projects.
// Inputs: the qualified route model, the parsed plugin/hand-managed snippets,
// and the Set of registrations read from disk.
// Outputs: sharedFacts (what this project serves) and an *errs.CLIError.
// Constraints: a bare host is ambiguous only when it is one label (a Docker
// service alias); names with a dot, IP literals and localhost resolve the same
// everywhere and are never refused. Hand-managed routes of a fronted project
// are the owner's own files and are not this project's.

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/nginx/routemodel"
	"github.com/nself-org/cli/internal/sharedproxy"
)

// sharedFacts is what this project serves on the proxy.
type sharedFacts struct {
	names    []string // server names, lower case
	verbatim []string // upstream hosts the build leaves as written
	certDirs []string // certificate directories the site confs reference
}

// certDirRE finds the certificate directory of an ssl_certificate line.
var certDirRE = regexp.MustCompile(`ssl_certificate(?:_key)?\s+\S*/certificates/([A-Za-z0-9][A-Za-z0-9._-]*)/`)

// collect gathers the facts from the model and the snippets.
func (sp *sharedProxy) collect(m *routemodel.Model, set *snippetSet) sharedFacts {
	names, verbatim, certs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	ours := map[string]bool{}
	for _, c := range sp.container {
		ours[c] = true
	}
	add := func(routes []routemodel.Route, generated bool) {
		for _, r := range routes {
			if r.Source == "hand_managed" && sp.role == sharedFronted {
				continue // the owner's own files
			}
			if r.ShadowedBy == nil {
				for _, n := range r.ServerNames {
					names[strings.ToLower(n)] = true
				}
			}
			if r.TLS != nil && r.TLS.SSLDir != "" && generated {
				certs[r.TLS.SSLDir] = true
			}
			for _, loc := range r.Locations {
				if loc.Upstream == nil {
					continue
				}
				h := loc.Upstream.Host
				switch r.Source {
				case "core", "optional", "custom_service":
					if !ours[h] {
						verbatim[h] = true
					}
				case "plugin":
					if _, bare := sp.container[h]; !bare && !ours[h] {
						verbatim[h] = true
					}
				default:
					verbatim[h] = true
				}
			}
		}
	}
	add(m.Routes, true)
	add(set.routes, false)
	for _, f := range set.files {
		if f.source != "plugin" {
			continue
		}
		for _, g := range certDirRE.FindAllStringSubmatch(f.text, -1) {
			certs[g[1]] = true
		}
	}
	return sharedFacts{names: keys(names), verbatim: keys(verbatim), certDirs: keys(certs)}
}

// keys returns the sorted keys of a set.
func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// others returns every registration this project must not collide with: the
// owner's file (for a fronted project) and the other projects'.
func (sp *sharedProxy) others() []sharedproxy.Registration {
	var out []sharedproxy.Registration
	if sp.role == sharedFronted && sp.set.Owner != nil {
		out = append(out, *sp.set.Owner)
	}
	return append(out, sp.set.Others(sp.project)...)
}

// ambiguousHost reports whether h is a bare Docker alias (one label, not an
// IP and not localhost).
func ambiguousHost(h string) bool {
	return h != "" && !strings.Contains(h, ".") && h != "localhost" && net.ParseIP(h) == nil
}

// check refuses E503 (a name or certificate directory held by another
// project) and E504 (an ambiguous bare upstream host) before any write.
func (sp *sharedProxy) check() error {
	for _, other := range sp.others() {
		for _, n := range sp.facts.names {
			if inSorted(other.ServerNames, n) {
				return errs.New("E503", fmt.Sprintf("%s is served by project %q (%s) and project %q would serve it too", n, other.Project, other.ProjectDir, sp.project)).
					WithFix("Give one of the two projects another domain. Nothing was written.")
			}
		}
		for _, d := range sp.facts.certDirs {
			prefix := "ssl/certificates/" + d + "/"
			for _, f := range other.Files {
				if strings.HasPrefix(f, prefix) {
					return errs.New("E503", fmt.Sprintf("certificate directory %s is held by project %q and project %q would use it too", d, other.Project, sp.project)).
						WithFix("Use another certificate directory name for one of the two projects. Nothing was written.")
				}
			}
		}
		for _, h := range sp.facts.verbatim {
			if !ambiguousHost(h) || !(inSorted(other.ServiceNames, h) || inSorted(other.UpstreamHosts, h)) {
				continue
			}
			msg := fmt.Sprintf("upstream host %q is left as written by project %q and is also a service or upstream host of project %q (%s)", h, sp.project, other.Project, other.ProjectDir)
			if sp.role == sharedOwner {
				// The owner leaves plugin and hand-managed upstreams as written (EPIC D2), and every
				// plugin is installed for every project on the host, so refusing here would
				// block the owner as soon as a fronted project runs the same plugin.
				warnOnce("shared proxy: " + msg + "; use a container name in this route to avoid reaching the wrong project")
				continue
			}
			return errs.New("E504", msg).
				WithFix("Point the route at a container name (<PROJECT_NAME>_<service>) or at a host name no other project uses. Nothing was written.")
		}
	}
	return nil
}
