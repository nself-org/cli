// Registry fragment: shared host proxy runtime code E506, owned by
// P7-ADOPT-21 (EPIC ADOPT D2, contract:cli.shared-proxy). Registered from
// init() through Register; see codes.go for the rules and codes_blocks.go for
// the allocation. The producer is internal/sharedproxy (attach.go).
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E506",
			Summary:    "Shared proxy: attaching the owner's nginx failed",
			DefaultWhy: "The owner's nginx could not be connected to this project's network, `nginx -t` rejected the combined configuration, or the reload failed. The owner keeps serving its previous configuration and the network is disconnected again. When the test named a project's files, those files were moved to nginx/shared/quarantine/<project>/ in the owner's directory.",
			DefaultFix: "Read the nginx error above, fix the named conf in the project that owns it, and run `nself build && nself start` there again; a successful start restores quarantined files. If the owner's nginx container is not running, start the owner first.",
			DocsPath:   "reference/error-codes#e506",
			Exit:       ExitUserError,
		},
	)
}
