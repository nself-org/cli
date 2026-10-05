package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/output"
)

func TestExecutePassesThroughNormalResults(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if err := execute(func() error { return nil }); err != nil {
			t.Fatalf("nil became %v", err)
		}
		want := errors.New("plain")
		if err := execute(func() error { return want }); !errors.Is(err, want) {
			t.Fatalf("error not passed through: %v", err)
		}
	})
}

func TestExecuteRecoversPanicInV15Only(t *testing.T) {
	compattest.Set(t, true)
	err := execute(func() error { panic("kaboom") })
	if err == nil || !strings.Contains(err.Error(), "internal error: kaboom") {
		t.Fatalf("panic not recovered into an error: %v", err)
	}
	output.ResetState()
	t.Cleanup(output.ResetState)
	if code, _, errOut := run(err); code != 1 || !strings.HasPrefix(errOut, "Error: internal error: kaboom") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}

	// v1.4: the panic propagates untouched.
	compattest.Set(t, false)
	defer func() {
		if r := recover(); r != "kaboom" {
			t.Fatalf("v1.4 must not recover, got %v", r)
		}
	}()
	_ = execute(func() error { panic("kaboom") })
	t.Fatal("unreachable: v1.4 panic should have propagated")
}
