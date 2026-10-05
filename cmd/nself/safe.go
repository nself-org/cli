package main

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// execute runs the command tree. In v1.5 mode a panic that escapes it is
// recovered into an ordinary failure, so the process still goes through report:
// one error envelope in JSON mode, an "Error: internal error: ..." line on
// stderr, and exit status 2 (infra class), the same status a Go crash gives, so
// scripts see no change in status. The goroutine stack trace is written to
// stderr before the error line for bug reports; it never enters the JSON
// envelope. The panic value is carried in the error message only (report redacts it);
// the trace block never prints the value. In v1.4 mode the panic propagates exactly as before.
func execute(run func() error, stderr io.Writer) (err error) {
	// compat.V15(P7-REG-06): a panic crashes the process with the raw Go runtime trace and exit 2 -> the panic is reported as an "internal error" (error envelope in JSON mode, stack trace on stderr only), still exit 2
	if !compat.V15() {
		return run()
	}
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(stderr, "panic recovered; stack trace:\n%s\n", debug.Stack())
			err = errs.ExitWith(errs.ExitInfraError, fmt.Errorf("internal error: %v", r))
		}
	}()
	return run()
}
