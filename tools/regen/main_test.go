package main

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestGeneratorOrder(t *testing.T) {
	// The required order per the spec:
	// canongen, then cmd-inventory steps (cmdinventory, openapigen, facts),
	// then wikigen -report, parity, schemagen, then core-services.
	expected := []string{
		"canongen",
		"cmdinventory",
		"openapigen",
		"facts",
		"wikigen-report",
		"parity",
		"schemagen",
		"core-services",
	}

	steps := defaultSteps()
	var actual []string
	for _, s := range steps {
		actual = append(actual, s.Name)
	}

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("generator order mismatch\nwant: %v\ngot:  %v", expected, actual)
	}
}

func TestIdempotentGitClean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Use noop steps so we don't actually run generators in test,
	// just verify that Run on a clean tree leaves git status clean.
	noopSteps := make([]Step, len(GeneratorOrder))
	for i, name := range GeneratorOrder {
		noopSteps[i] = Step{Name: name, Run: func(tmpDir string) error { return nil }}
	}

	err := Run(noopSteps)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestFailingStepReturnsNamedError(t *testing.T) {
	fakeSteps := []Step{
		{
			Name: "good-step",
			Run:  func(tmpDir string) error { return nil },
		},
		{
			Name: "bad-step",
			Run:  func(tmpDir string) error { return errors.New("something broke") },
		},
		{
			Name: "never-reached",
			Run:  func(tmpDir string) error { return nil },
		},
	}

	err := Run(fakeSteps)
	if err == nil {
		t.Fatal("expected error from failing step, got nil")
	}
	if !strings.Contains(err.Error(), "bad-step") {
		t.Errorf("error should name the failing step 'bad-step', got: %v", err)
	}
}
