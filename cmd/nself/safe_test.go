package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/output"
)

func TestExecutePassesThroughNormalResults(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if err := execute(func() error { return nil }, io.Discard); err != nil {
			t.Fatalf("nil became %v", err)
		}
		want := errors.New("plain")
		if err := execute(func() error { return want }, io.Discard); !errors.Is(err, want) {
			t.Fatalf("error not passed through: %v", err)
		}
	})
}

func TestExecuteRecoversPanicInV15Only(t *testing.T) {
	compattest.Set(t, true)
	secret := "pw" + "Sup3rS3cret"
	var trace bytes.Buffer
	err := execute(func() error { panic("kaboom password=" + secret) }, &trace)
	if err == nil || !strings.Contains(err.Error(), "internal error: kaboom") {
		t.Fatalf("panic not recovered into an error: %v", err)
	}
	if !strings.Contains(trace.String(), "goroutine ") || !strings.Contains(trace.String(), "TestExecuteRecoversPanicInV15Only") {
		t.Fatalf("stack trace missing from the stderr writer:\n%s", trace.String())
	}

	if strings.Contains(trace.String(), secret) {
		t.Fatalf("trace block prints the panic value:\n%s", trace.String())
	}

	// Human mode: exit 2 (infra), message on stderr, redacted.
	output.ResetState()
	t.Cleanup(output.ResetState)
	code, out, errOut := run(err)
	if code != 2 || out != "" || !strings.HasPrefix(errOut, "Error: internal error: kaboom") || strings.Contains(errOut, secret) {
		t.Fatalf("human: code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	// JSON mode: one envelope, class infra, exit_code 2, and no stack trace.
	output.SetInvocation("start", true)
	code, out, _ = run(err)
	doc := decodeOne(t, out)
	e, _ := doc["error"].(map[string]any)
	if code != 2 || e["exit_code"] != float64(2) || e["class"] != "infra" || e["code"] != "E400" {
		t.Fatalf("json: code=%d doc=%v", code, doc)
	}
	if strings.Contains(out, "goroutine") || strings.Contains(out, "debug.Stack") || strings.Contains(out, secret) {
		t.Fatalf("envelope leaks trace or secret:\n%s", out)
	}

	// v1.4: the panic propagates untouched and nothing is written.
	compattest.Set(t, false)
	trace.Reset()
	defer func() {
		if r := recover(); r != "kaboom" {
			t.Fatalf("v1.4 must not recover, got %v", r)
		}
		if trace.Len() != 0 {
			t.Fatalf("v1.4 wrote %q", trace.String())
		}
	}()
	_ = execute(func() error { panic("kaboom") }, &trace)
	t.Fatal("unreachable: v1.4 panic should have propagated")
}
