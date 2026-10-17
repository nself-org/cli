package build

// shared_proxy_qualified.go — the container names of another registered project.
//
// Purpose: the E504 check refused only a bare service name another project
// registered. The shared nginx is attached to every registered project's
// network, so a snippet that proxies to `<other-project>_<service>` reaches
// that project's private service just as well. This file names those hosts.
// Inputs: another project's registration (Project, ServiceNames).
// Outputs: whether a verbatim upstream host is one of its container names.
// Constraints: the naming rule is config.ServiceContainerName's
// (`<PROJECT_NAME>_<service>`, `nself-admin` is `admin`); this project's own
// container names never match, because only other projects are asked.

import (
	"strings"

	"github.com/nself-org/cli/internal/sharedproxy"
)

// isContainerOf reports whether host (already normalised) is the container
// name of one of other's registered services.
func isContainerOf(host string, other sharedproxy.Registration) bool {
	prefix := strings.ToLower(other.Project) + "_"
	if !strings.HasPrefix(host, prefix) {
		return false
	}
	svc := strings.TrimPrefix(host, prefix)
	for _, s := range other.ServiceNames {
		if s == "nself-admin" {
			s = "admin"
		}
		if strings.ToLower(strings.TrimSpace(s)) == svc {
			return true
		}
	}
	return false
}
