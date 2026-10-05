package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/nself-org/cli/internal/errs"
)

// reportLegacy is the v1.4 error path, moved verbatim from the previous
// main.go: the status is the error's own ExitCode() or 1, a silent error
// prints nothing, everything else prints exactly "Error: <err>" on stderr
// (no redaction, no code block, no JSON envelope). The old structured-error
// branch is gone because nothing in the tree ever produced one (it had no
// callers outside its own tests). stdout is unused and stays untouched.
func reportLegacy(err error, _, stderr io.Writer) int {
	code := legacyExitCode(err)

	// A silent error means the command (or a plugin subprocess sharing this
	// terminal) already wrote its output; printing again would duplicate it.
	var silencer errs.Silencer
	if errors.As(err, &silencer) && silencer.Silent() {
		return code
	}

	_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
	return code
}

// legacyExitCode is the v1.4 status: an explicit ExitCode() wins, else 1.
func legacyExitCode(err error) int {
	var coder errs.ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return 1
}
