package config

// container_names.go — the container name of a compose service.
//
// Purpose: two projects behind one nginx both answer to `hasura` on their own
// networks, so a shared proxy addresses services by container name, which is
// unique per host (EPIC ADOPT D2). Compose already names every container
// `<PROJECT_NAME>_<service>`; this is that rule in one place.
// Inputs: the compose service name (the name DetectServices returns).
// Outputs: the container name.
// Constraints: the admin service is named `nself-admin` in compose and its
// container is `<PROJECT_NAME>_admin` (internal/compose optional_services_apps.go);
// that is the only service whose container suffix differs from its name.

import "strings"

// ServiceContainerName returns the container name of compose service svc:
// `<PROJECT_NAME>_<svc>`.
func (c *Config) ServiceContainerName(svc string) string {
	if svc == "nself-admin" {
		svc = "admin"
	}
	return c.ProjectName + "_" + strings.TrimSpace(svc)
}
