package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// errDiskFull is the error the failing writer returns.
var errDiskFull = errors.New("disk full")

// failWriter fails every Write, so a swallowed write error shows up as a nil
// return from the Emit function under test.
type failWriter struct{ calls int }

func (f *failWriter) Write([]byte) (int, error) { f.calls++; return 0, errDiskFull }

func TestWriteErrorsPropagate(t *testing.T) {
	reset(t)
	t.Run("EmitData", func(t *testing.T) {
		fw := &failWriter{}
		if err := EmitData(Writer{Out: fw, Err: &bytes.Buffer{}}, "x", 1); !errors.Is(err, errDiskFull) {
			t.Fatalf("got %v, want the writer's error", err)
		}
		if fw.calls != 1 {
			t.Fatalf("document written in %d calls, want 1", fw.calls)
		}
	})
	t.Run("EmitError", func(t *testing.T) {
		if err := EmitError(Writer{Out: &failWriter{}}, "x", errors.New("boom")); !errors.Is(err, errDiskFull) {
			t.Fatalf("got %v, want the writer's error", err)
		}
	})
	compattest.Both(t, func(t *testing.T) {
		t.Run("EmitLegacyCompatible", func(t *testing.T) {
			reset(t)
			t.Setenv(LegacyEnvVar, "")
			if err := EmitLegacyCompatible(Writer{Out: &failWriter{}}, "x", 1); !errors.Is(err, errDiskFull) {
				t.Fatalf("got %v, want the writer's error", err)
			}
		})
	})
}

func TestLegacyWarningNotPrintedWhenBareWriteFails(t *testing.T) {
	compattest.Set(t, true)
	reset(t)
	t.Setenv(LegacyEnvVar, "1")
	var errb bytes.Buffer
	if err := EmitLegacyCompatible(Writer{Out: &failWriter{}, Err: &errb}, "x", 1); !errors.Is(err, errDiskFull) {
		t.Fatalf("got %v, want the writer's error", err)
	}
	if errb.Len() != 0 {
		t.Fatalf("warning printed for a document that was not written: %q", errb.String())
	}
	// The failed call must not have used up the once-per-process warning.
	w, _, errb2 := bufs()
	if err := EmitLegacyCompatible(w, "x", 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errb2.String(), "NSELF_JSON_LEGACY") {
		t.Fatalf("warning lost after a failed write: %q", errb2.String())
	}
}
