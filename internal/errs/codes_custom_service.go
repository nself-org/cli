// Registry fragment: custom-service (CS_N v2) codes E500-E502 and E528, owned
// by P7-ADOPT-01. Registered from init() through Register; see codes.go for
// the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E500",
			Summary:    "Custom service dependency is not valid",
			DefaultWhy: "CS_N_DEPENDS_ON names a service that does not exist in this project, or uses a condition other than started, healthy or completed.",
			DefaultFix: "Use name[:started|healthy|completed], comma-separated, and name a core service, another custom service or a service from an installed plugin.",
			DocsPath:   "reference/error-codes#e500",
			Exit:       ExitUserError,
		},
		CodeEntry{
			Code:       "E501",
			Summary:    "Custom service network is not allowed",
			DefaultWhy: "CS_N_NETWORKS names a network that does not start with <PROJECT_NAME>_. A custom service never joins another project's network.",
			DefaultFix: "Rename the network to <PROJECT_NAME>_<name> (lowercase letters, digits, - and _), or remove it from CS_N_NETWORKS.",
			DocsPath:   "reference/error-codes#e501",
			Exit:       ExitUserError,
		},
		CodeEntry{
			Code:       "E502",
			Summary:    "Custom service host bind is not allowed",
			DefaultWhy: "CS_N_VOLUMES binds a writable absolute host path outside the project, or the Docker socket, or the host root. These give the container control of the host.",
			DefaultFix: "Use a named volume or a project-relative path, mount an outside path read-only (:ro), and never mount /var/run/docker.sock or /.",
			DocsPath:   "reference/error-codes#e502",
			Exit:       ExitUserError,
		},
		CodeEntry{
			Code:       "E528",
			Summary:    "Custom service build context is not allowed",
			DefaultWhy: "CS_N_PATH reaches above the repository root, or an ancestor build context has no .dockerignore that excludes .env* and .secrets. The whole directory is sent to the image builder.",
			DefaultFix: "Point CS_N_PATH at the project or at an ancestor no higher than the directory that holds .git, and add .env* and .secrets/ to that directory's .dockerignore.",
			DocsPath:   "reference/error-codes#e528",
			Exit:       ExitUserError,
		},
	)
}
