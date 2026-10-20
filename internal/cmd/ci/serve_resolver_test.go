package ci

// Purpose: RunServe must refuse to start without a verified gate resolver.
// Inputs: a ServeConfig with a secret and a temp workdir, with and without ResolveGate.
// Outputs: assertions on the error RunServe returns before it listens.
// Constraints: no network; the resolver failure paths return before the listener opens.

import (
	"errors"
	"strings"
	"testing"
)

func TestRunServeRequiresGateResolver(t *testing.T) {
	base := ServeConfig{Secret: "fixture-secret", WorkDir: t.TempDir(), AllowedRepos: []string{"o/r"}}

	err := RunServe(base)
	if err == nil || !strings.Contains(err.Error(), "no gate resolver configured") {
		t.Fatalf("nil resolver: got %v, want a refusal naming the missing resolver", err)
	}

	boom := errors.New("not verified")
	base.ResolveGate = func(bool) (string, error) { return "", boom }
	err = RunServe(base)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "nself-ci binary unavailable") {
		t.Fatalf("resolver error: got %v, want it wrapped as unavailable", err)
	}
}
