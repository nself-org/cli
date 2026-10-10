// Registry fragment: plugin resolution and compose-fragment codes (E128-E131),
// owned by P7-PLUG-17. Registered from init() through Register; see codes.go
// for the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E128",
			Category:   "plugin",
			Summary:    "Compose plugin has no compose fragment",
			DefaultWhy: "The plugin declares a compose service (service.kind compose, or a Dockerfile and a port), but it ships no docker-compose.plugin.yml, so nself build would silently leave it out of the stack.",
			DefaultFix: "Reinstall or update the plugin so its docker-compose.plugin.yml is present, or ask the plugin author to mark it service.kind cli or library if it runs no container.",
			DocsPath:   "reference/error-codes#e128",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E129",
			Category:   "plugin",
			Summary:    "Plugin fragment uses a foreign external network",
			DefaultWhy: "The plugin's docker-compose.plugin.yml attaches a service to an external network other than the project network, which would reach outside this stack.",
			DefaultFix: "Attach the service to ${DOCKER_NETWORK} only, or remove the plugin; ask the plugin author to drop the external network from the fragment.",
			DocsPath:   "reference/error-codes#e129",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E130",
			Category:   "plugin",
			Summary:    "plugin compose fragment violates policy (ADR 0027)",
			DefaultWhy: "A plugin compose fragment asks for something the plugin policy forbids, such as mounting the Docker socket.",
			DefaultFix: "Remove the offending setting from the plugin's docker-compose.plugin.yml, or remove the plugin.",
			DocsPath:   "reference/error-codes#e130",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E131",
			Category:   "plugin",
			Summary:    "Plugin tier change needs --tier",
			DefaultWhy: "An update would move an installed plugin to another tier (free or licensed), and nself never changes the tier of an existing install on its own.",
			DefaultFix: "Re-run with --tier free or --tier licensed to choose the tier on purpose, or keep the installed tier by leaving the plugin as it is.",
			DocsPath:   "reference/error-codes#e131",
			Exit:       1,
		},
	)
}
