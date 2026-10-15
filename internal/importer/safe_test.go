package importer

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestAllowMessagesSurfaceOnlyRegisteredText(t *testing.T) {
	const fixed = "source fixture refused the thing"
	AllowMessages(fixed)
	if got := publicError(errs.New("E517", fixed), "E517", "generic"); !strings.Contains(got.Error(), fixed) {
		t.Fatalf("registered text must pass through: %v", got)
	}
	got := publicError(errs.New("E517", "source fixture refused password hunter2"), "E517", "generic")
	if strings.Contains(got.Error(), "hunter2") || !strings.Contains(got.Error(), "generic") {
		t.Fatalf("unregistered text must be replaced: %v", got)
	}
	if got = publicError(errs.New("E519", fixed), "E517", "generic"); strings.Contains(got.Error(), fixed) {
		t.Fatalf("only E517/E518/E520/E521 may pass text through: %v", got)
	}
}

func TestSafeErrorKeepsItsFixedText(t *testing.T) {
	const fixed = "project has no Firestore collections"
	got := publicError(NewSafe("E520", fixed), "E520", "generic")
	if !strings.Contains(got.Error(), fixed) || strings.Contains(got.Error(), "generic") {
		t.Fatalf("safe text must pass through: %v", got)
	}
	if got = publicError(NewSafe("E519", fixed), "E520", "generic"); strings.Contains(got.Error(), fixed) {
		t.Fatalf("only E517/E518/E520/E521 may pass text through: %v", got)
	}
	if got = publicError(NewSafe("E520", "bad\x1b[31m text"), "E520", "generic"); strings.Contains(got.Error(), "\x1b") {
		t.Fatalf("control characters must not reach the terminal: %v", got)
	}
}
