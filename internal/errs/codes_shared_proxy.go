// Registry fragment: shared host proxy codes E503-E505, owned by P7-ADOPT-03
// (EPIC ADOPT D2, contract:cli.shared-proxy). Registered from init() through
// Register; see codes.go for the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E503",
			Summary:    "Shared proxy: name or certificate already served by another project",
			DefaultWhy: "A server name or certificate directory this project would put on the shared host proxy is already served by the proxy's owner or by another registered project, or the owner's ssl/ directory cannot be written safely (a symlink, or no permission).",
			DefaultFix: "Give this project its own domain or certificate directory, or remove the other project's registration with `nself uninstall` in its directory. Never replace a symlink in the owner's ssl/ by hand.",
			DocsPath:   "reference/error-codes#e503",
			Exit:       ExitUserError,
		},
		CodeEntry{
			Code:       "E504",
			Summary:    "Shared proxy: upstream host is ambiguous across projects",
			DefaultWhy: "A route upstream host that this build leaves as written is the name of a service, or a verbatim upstream host, of the proxy's owner or of another registered project. On one shared nginx that name could reach the wrong project.",
			DefaultFix: "Point the route at a container name (`<PROJECT_NAME>_<service>`) or at a host name that is not a service of another project.",
			DocsPath:   "reference/error-codes#e504",
			Exit:       ExitUserError,
		},
		CodeEntry{
			Code:       "E505",
			Summary:    "NGINX_MODE is not valid here",
			DefaultWhy: "NGINX_MODE is set to a value other than shared, or NGINX_MODE=shared is combined with NGINX_FRONTED_BY, or the project's proxy provider is not nginx.",
			DefaultFix: "Set NGINX_MODE=shared only on the project that owns the host nginx, with no NGINX_FRONTED_BY and nginx as the proxy provider, or unset NGINX_MODE.",
			DocsPath:   "reference/error-codes#e505",
			Exit:       ExitUserError,
		},
	)
}
