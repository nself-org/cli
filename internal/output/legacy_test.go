package output

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/ui"
)

// uiPrintJSON captures what ui.PrintJSON writes to os.Stdout for v.
func uiPrintJSON(t *testing.T, v any) []byte {
	t.Helper()
	r, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = wr
	perr := ui.PrintJSON(v)
	os.Stdout = old
	wr.Close()
	got, rerr := io.ReadAll(r)
	r.Close()
	if perr != nil || rerr != nil {
		t.Fatalf("capture: %v %v", perr, rerr)
	}
	return got
}

func TestLegacyCompatibleBothModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		reset(t)
		t.Setenv(LegacyEnvVar, "")
		w, out, errb := bufs()
		if err := EmitLegacyCompatible(w, "status", sampleValue); err != nil {
			t.Fatal(err)
		}
		if errb.Len() != 0 {
			t.Fatalf("wrote to Err: %q", errb.String())
		}
		if strings.HasSuffix(t.Name(), "v1.4") {
			if want := uiPrintJSON(t, sampleValue); !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("v1.4 not byte-identical to ui.PrintJSON\n%q\n%q", out.Bytes(), want)
			}
			return
		}
		ref, refOut, _ := bufs()
		if err := EmitData(ref, "status", sampleValue); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out.Bytes(), refOut.Bytes()) {
			t.Fatalf("v1.5 is not the envelope:\n%s", out.String())
		}
		if !strings.Contains(out.String(), `"schema_version": "1"`) {
			t.Fatalf("no envelope: %s", out.String())
		}
	})
}

func TestLegacyBareKeepsHTMLEscaping(t *testing.T) {
	compattest.Set(t, false)
	reset(t)
	w, out, _ := bufs()
	if err := EmitLegacyCompatible(w, "status", sampleValue); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `\u003ckey\u003e \u0026 value`) {
		t.Fatalf("bare output must keep json.MarshalIndent escaping: %s", out.String())
	}
}

func TestLegacyEnvInV15(t *testing.T) {
	compattest.Set(t, true)
	reset(t)
	t.Setenv(LegacyEnvVar, "1")
	w, out, errb := bufs()
	for i := 0; i < 2; i++ {
		out.Reset()
		if err := EmitLegacyCompatible(w, "status", sampleValue); err != nil {
			t.Fatal(err)
		}
		if want := uiPrintJSON(t, sampleValue); !bytes.Equal(out.Bytes(), want) {
			t.Fatalf("call %d: not byte-identical to ui.PrintJSON\n%q\n%q", i, out.Bytes(), want)
		}
	}
	const warn = "warning: NSELF_JSON_LEGACY is deprecated and will be removed in v1.6.0; parse the v1 envelope instead (see JSON-Output wiki page)\n"
	if errb.String() != warn {
		t.Fatalf("want the warning exactly once, got %q", errb.String())
	}
}

func TestLegacyEnvDoesNotTouchEmitDataOrError(t *testing.T) {
	compattest.Set(t, true)
	reset(t)
	t.Setenv(LegacyEnvVar, "1")
	w, out, errb := bufs()
	if err := EmitData(w, "status", sampleValue); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"schema_version": "1"`) {
		t.Fatalf("EmitData not enveloped: %s", out.String())
	}
	out.Reset()
	if err := EmitError(w, "status", errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"error"`) || !strings.Contains(out.String(), errs.Describe(errors.New("boom")).Code) {
		t.Fatalf("EmitError not enveloped: %s", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("warning printed without a legacy emit: %q", errb.String())
	}
}

func TestLegacyEnvIgnoredInV14(t *testing.T) {
	compattest.Set(t, false)
	reset(t)
	t.Setenv(LegacyEnvVar, "1")
	w, _, errb := bufs()
	if err := EmitLegacyCompatible(w, "status", sampleValue); err != nil {
		t.Fatal(err)
	}
	if errb.Len() != 0 {
		t.Fatalf("v1.4 mode printed %q", errb.String())
	}
}

func TestLegacyWarningAgainAfterReset(t *testing.T) {
	compattest.Set(t, true)
	reset(t)
	t.Setenv(LegacyEnvVar, "1")
	w, _, errb := bufs()
	_ = EmitLegacyCompatible(w, "s", 1)
	ResetState()
	_ = EmitLegacyCompatible(w, "s", 1)
	if n := strings.Count(errb.String(), "NSELF_JSON_LEGACY"); n != 2 {
		t.Fatalf("want 2 warnings across a reset, got %d", n)
	}
}

func TestLegacyMarshalFailureIsReturned(t *testing.T) {
	for _, on := range []bool{false, true} {
		compattest.Set(t, on)
		reset(t)
		t.Setenv(LegacyEnvVar, "1")
		w, out, errb := bufs()
		if err := EmitLegacyCompatible(w, "s", make(chan int)); err == nil {
			t.Fatalf("v15=%v: nil error for unmarshalable data", on)
		}
		if out.Len() != 0 || errb.Len() != 0 {
			t.Fatalf("v15=%v: wrote %q / %q on failure", on, out.String(), errb.String())
		}
	}
}
