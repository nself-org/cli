package output

import (
	"fmt"
	"io"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/ui"
)

// codeUnclassified is the fallback code errs.Describe gives an error with no
// code of its own; such an error prints as a plain line.
const codeUnclassified = "E400"

// RenderError writes the human form of err to w (stderr in practice).
//
// An error that carries a code other than E400 (a *errs.CLIError or a wrapped
// coded sentinel) prints
//
//	Error: [CODE] message
//	  Why: <cause>
//	  Fix: <remediation>
//	  Docs: <url>
//
// where the three indented lines appear only when non-empty. Every other
// error prints exactly `Error: <err.Error()>`, as main did before. Colour comes
// from ui.C only, which honours NO_COLOR and TTY detection; the plain line is
// never coloured. A nil err writes nothing. Write errors are ignored: there is
// no better stream to report a failure to write the error stream.
func RenderError(w io.Writer, err error) {
	d := errs.Describe(err)
	if d == nil {
		return
	}
	if d.Code == codeUnclassified {
		fmt.Fprintf(w, "Error: %s\n", err.Error())
		return
	}
	fmt.Fprintf(w, "%s %s %s\n", ui.C(ui.BrightRed, "Error:"), ui.C(ui.Bold, "["+d.Code+"]"), d.Message)
	line := func(label, text string) {
		if text != "" {
			fmt.Fprintf(w, "  %s %s\n", ui.C(ui.Dim, label), text)
		}
	}
	line("Why:", d.Cause)
	line("Fix:", d.Remediation)
	if d.DocsURL != "" {
		fmt.Fprintf(w, "  %s %s\n", ui.C(ui.Dim, "Docs:"), ui.C(ui.Underline, d.DocsURL))
	}
}
