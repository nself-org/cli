package plugin

// installer_hint.go — the "run nself build" hint printed after an install.
//
// Purpose: print the post-install build hint only when the caller will not
// reconcile generated state itself. In v1.5 `nself plugin install`, `nself add`
// and `nself bundle install` reconcile right after the install, so the hint
// would tell the user to do something already done.
// Inputs: the install context (optionally marked by WithCallerReconciles) and
// the plugin name.
// Outputs: one hint line on hintOut (stderr), or nothing.
// Constraints: callers that do not reconcile (plugin.Update, the MCP install
// tool) keep the hint. The hint is advisory and never fails an install.

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/nself-org/cli/internal/compat"
)

// hintOut is where the build hint is written; a seam so tests can capture it.
// Nil means os.Stderr, read at print time so a swapped os.Stderr still sees it.
var hintOut io.Writer

// callerReconcilesKey is the unexported context key for WithCallerReconciles.
type callerReconcilesKey struct{}

// WithCallerReconciles marks ctx as belonging to a caller that reconciles
// generated state right after the install, so the build hint is redundant in
// v1.5. Callers that do not reconcile must not use it.
func WithCallerReconciles(ctx context.Context) context.Context {
	return context.WithValue(ctx, callerReconcilesKey{}, true)
}

// callerReconciles reports whether ctx was marked by WithCallerReconciles.
func callerReconciles(ctx context.Context) bool {
	v, _ := ctx.Value(callerReconcilesKey{}).(bool)
	return v
}

// printBuildHint prints the "run nself build" hint for the installed plugin,
// unless v1.5 is on and the caller reconciles after the install.
func printBuildHint(ctx context.Context, name string) {
	// compat.V15(D-0274): installer prints 'run nself build' after every install -> no hint when the caller reconciles
	if compat.V15() && callerReconciles(ctx) {
		return
	}
	out := hintOut
	if out == nil {
		out = os.Stderr
	}
	_, _ = fmt.Fprintf(out, "\nℹ Run 'nself build' to include %s in your stack.\n", name)
}
