package output

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// withTerminal forces the terminal decision for the test.
func withTerminal(t *testing.T, on bool) {
	t.Helper()
	orig := isTerminal
	t.Cleanup(func() { isTerminal = orig })
	isTerminal = func(io.Writer) bool { return on }
}

func codedErr() error { return &errs.CLIError{Code: "E057", What: "weak", Why: "short", Fix: "longer"} }

func TestColourFollowsTheWrittenStream(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	withTerminal(t, true)
	if !strings.Contains(string(render(codedErr())), "\x1b[") {
		t.Fatal("no colour although the stream is a terminal")
	}
	withTerminal(t, false)
	if strings.Contains(string(render(codedErr())), "\x1b[") {
		t.Fatal("colour although the stream is not a terminal")
	}
}

func TestNoColorWinsOverTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	withTerminal(t, true)
	if strings.Contains(string(render(codedErr())), "\x1b[") {
		t.Fatal("colour although NO_COLOR is set")
	}
}

func TestPlainErrorIsNeverColoured(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	withTerminal(t, true)
	if got := string(render(errors.New("boom"))); got != "Error: boom\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRealPipeAndBufferAreNotTerminals(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(w) || isTerminal(&bytes.Buffer{}) {
		t.Fatal("pipe or buffer reported as terminal")
	}
}
