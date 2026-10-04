// Registry fragment: docker codes (E001-E049). Registered from init() through
// Register; see codes.go for the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E001",
			Category:   "docker",
			Summary:    "Docker not installed",
			DefaultWhy: "The docker binary was not found in PATH.",
			DefaultFix: "Install Docker: https://docs.docker.com/get-docker/",
			DocsPath:   "reference/error-codes#e001",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E002",
			Category:   "docker",
			Summary:    "Docker daemon not running",
			DefaultWhy: "The Docker daemon is not responding to commands.",
			DefaultFix: "Start Docker Desktop or run: sudo systemctl start docker",
			DocsPath:   "reference/error-codes#e002",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E003",
			Category:   "docker",
			Summary:    "Docker Compose not available",
			DefaultWhy: "docker compose v2 plugin is not installed.",
			DefaultFix: "Update Docker Desktop or install the compose plugin manually.",
			DocsPath:   "reference/error-codes#e003",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E004",
			Category:   "docker",
			Summary:    "docker-compose.yml not found",
			DefaultWhy: "No docker-compose.yml exists in the project directory.",
			DefaultFix: "Run 'nself build' to generate the compose file.",
			DocsPath:   "reference/error-codes#e004",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E005",
			Category:   "docker",
			Summary:    "Port conflict",
			DefaultWhy: "A required port is already in use by another process.",
			DefaultFix: "Run 'nself doctor' to identify the conflict, then stop the conflicting process or change the port in .env.",
			DocsPath:   "reference/error-codes#e005",
			Exit:       2,
		},
	)
}
