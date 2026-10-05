package main

import (
	"errors"
	"io"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
)

// report maps the error returned by the command tree to a process exit status
// and writes everything the user and machine consumers see. It never calls
// os.Exit, so tests drive it in-process; main passes the result to os.Exit.
//
// v1.5 mode (contract:cli.exit-codes v1, contract:cli.json-envelope v1):
//
//   - the status comes from the exit-class classification (1 user, 2 infra,
//     3 auth, 4 destructive-blocked), the only mapping main uses;
//   - a silent error (the command or a subprocess already wrote its output)
//     prints nothing, in both modes;
//   - in JSON mode exactly one error envelope goes to stdout, so one parse of
//     stdout always yields data or error; its exit_code equals the return value.
//     The envelope carries the registry command path and the redacted error
//     detail, never raw argv. NSELF_JSON_LEGACY does not suppress it (EPIC D8);
//   - the human rendering (D9) always goes to stderr.
//
// v1.4 mode keeps origin/main's handling byte for byte (reportLegacy).
//
// args are the raw arguments after the program name. They are only scanned for
// --json when the failure happened before a command was resolved (unknown
// command or flag); their values are never written anywhere.
func report(err error, stdout, stderr io.Writer, args []string) int {
	// compat.V15(P7-REG-06): exit status 1 and a plain "Error: msg" line for every failure -> exit classes 2/3/4, "Error: [Exxx]" block on stderr and a JSON error envelope on stdout
	if !compat.V15() {
		return reportLegacy(err, stdout, stderr)
	}

	code := errs.ExitCodeFor(err)

	var silencer errs.Silencer
	if errors.As(err, &silencer) && silencer.Silent() {
		return code
	}

	command, jsonMode, known := output.Invocation()
	if !known {
		command, jsonMode = "", output.JSONRequestedFromArgs(args)
	}
	if jsonMode {
		// A failed write of the envelope has no better channel; the status
		// still tells the caller the command failed.
		_ = output.EmitError(output.Writer{Out: stdout, Err: stderr}, command, err)
	}
	output.RenderError(stderr, err)
	return code
}
