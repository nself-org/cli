package output

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func render(err error) []byte {
	var b bytes.Buffer
	RenderError(&b, err)
	return b.Bytes()
}

func TestRenderPlainError(t *testing.T) {
	if got := string(render(errors.New("boom"))); got != "Error: boom\n" {
		t.Fatalf("got %q", got)
	}
	// A wrapped plain error keeps the full chain text, as main did.
	err := fmt.Errorf("outer: %w", errors.New("inner"))
	if got := string(render(err)); got != "Error: outer: inner\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderCLIErrorGolden(t *testing.T) {
	ce := &errs.CLIError{
		Code: "E057", What: "password is insecure", Why: "it is on the common list",
		Fix: "set a longer password", DocsPath: "reference/error-codes#e057",
	}
	golden(t, "render-cli-error.golden.txt", stripANSI(render(ce)))
}

func TestRenderCLIErrorOmitsEmptyLines(t *testing.T) {
	ce := &errs.CLIError{Code: "E300", What: "init failed"}
	got := string(stripANSI(render(ce)))
	// E300 has a registry DocsPath but the CLIError has none: Describe falls
	// back to the registry, so only Why and Fix are absent.
	if got[:len("Error: [E300] init failed\n")] != "Error: [E300] init failed\n" {
		t.Fatalf("got %q", got)
	}
	if bytes.Contains([]byte(got), []byte("Why:")) || bytes.Contains([]byte(got), []byte("Fix:")) {
		t.Fatalf("empty Why/Fix printed: %q", got)
	}
}

func TestRenderWrappedSentinelGolden(t *testing.T) {
	err := fmt.Errorf("start: %w", errs.ErrDockerNotRunning)
	golden(t, "render-sentinel.golden.txt", stripANSI(render(err)))
}

func TestRenderNilWritesNothing(t *testing.T) {
	if got := render(nil); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestRenderExplicitE400IsPlain(t *testing.T) {
	ce := &errs.CLIError{Code: "E400", What: "something"}
	if got := string(render(ce)); got != "Error: "+ce.Error()+"\n" {
		t.Fatalf("got %q", got)
	}
}
