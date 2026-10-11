// Purpose: register HTTP API transport refusals E428-E432 (SURF D14).
// Inputs: none. Outputs: error registry entries.
// Constraints: classes and exits are part of cli.http-api v1.
package errs

func init() {
	Register(
		CodeEntry{Code: "E428", Category: "cli", Summary: "Host not allowed", DefaultWhy: "The request Host is outside the server's allowed hosts and port.", DefaultFix: "Use the configured listener host or pass --allowed-host explicitly.", DocsPath: "reference/error-codes#e428", Exit: ExitUserError},
		CodeEntry{Code: "E429", Category: "cli", Summary: "Origin not allowed", DefaultWhy: "The request Origin is outside the server's allowed HTTP origins.", DefaultFix: "Use an allowed HTTP origin on the listener port.", DocsPath: "reference/error-codes#e429", Exit: ExitUserError},
		CodeEntry{Code: "E430", Category: "cli", Summary: "Malformed HTTP request", DefaultWhy: "The method, JSON body, content type, body size or Accept header is invalid.", DefaultFix: "POST a JSON machine request no larger than 1 MiB and accept application/json or application/x-ndjson.", DocsPath: "reference/error-codes#e430", Exit: ExitUserError},
		CodeEntry{Code: "E431", Category: "cli", Summary: "Server saturated", DefaultWhy: "All machine invocation slots are in use.", DefaultFix: "Retry after one second.", DocsPath: "reference/error-codes#e431", Exit: ExitInfraError},
		CodeEntry{Code: "E432", Category: "cli", Summary: "No such route", DefaultWhy: "The registry has no command at this route.", DefaultFix: "Read /openapi.json and use a documented command route.", DocsPath: "reference/error-codes#e432", Exit: ExitUserError},
	)
}
