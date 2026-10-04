package output

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/ui"
)

// codeUnclassified is the fallback code errs.Describe gives an error with no
// code of its own; such an error prints as a plain line.
const codeUnclassified = "E400"

// isTerminal reports whether w is a terminal. It is a variable so tests can
// drive both colour branches without a pseudo-terminal.
var isTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// colorFor reports whether RenderError should colour its output for w: only
// when w itself is a terminal and NO_COLOR is unset. ui.C decides from the
// stdout terminal, which is wrong for a stream that may be redirected on its
// own (`nself x 2>err.log`, `nself x --json | jq`).
func colorFor(w io.Writer) bool {
	return os.Getenv("NO_COLOR") == "" && isTerminal(w)
}

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
// error prints exactly `Error: <message>`, as main did before. Message, cause
// and remediation are redacted (URL credentials, tokens, e-mail, IPs; see
// redactText), so a plain error changes only when it carries such a value.
// Colour is decided from w's own terminal state (colorFor); the plain line is
// never coloured. A nil err writes nothing. Write errors are ignored: there is
// no better stream to report a failure to write the error stream.
func RenderError(w io.Writer, err error) {
	d := errs.Describe(err)
	if d == nil {
		return
	}
	if d.Code == codeUnclassified {
		_, _ = fmt.Fprintf(w, "Error: %s\n", redactText(err.Error()))
		return
	}
	d = redactDetail(d)
	paint := func(code, text string) string {
		if colorFor(w) {
			return code + text + ui.Reset
		}
		return text
	}
	_, _ = fmt.Fprintf(w, "%s %s %s\n", paint(ui.BrightRed, "Error:"), paint(ui.Bold, "["+d.Code+"]"), d.Message)
	line := func(label, text string) {
		if text != "" {
			_, _ = fmt.Fprintf(w, "  %s %s\n", paint(ui.Dim, label), text)
		}
	}
	line("Why:", d.Cause)
	line("Fix:", d.Remediation)
	if d.DocsURL != "" {
		_, _ = fmt.Fprintf(w, "  %s %s\n", paint(ui.Dim, "Docs:"), paint(ui.Underline, d.DocsURL))
	}
}
