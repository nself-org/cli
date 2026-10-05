package main

import (
	"os"

	"github.com/nself-org/cli/cmd/commands"
)

// main is the only place in the tree allowed to call os.Exit. Commands signal a
// non-zero status by returning an error: errs.Exit / errs.ExitWith carry an
// explicit code, everything else is classified by report (contract:cli.exit-codes
// v1, internal/errs/exit_codes.go). execute turns a panic into an error first.
// Rendering and exit-status rules live in report.go and report_legacy.go.
func main() {
	if err := execute(commands.Execute); err != nil {
		os.Exit(report(err, os.Stdout, os.Stderr, os.Args[1:]))
	}
}
