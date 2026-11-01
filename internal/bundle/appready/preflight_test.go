package appready

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestPreflightStopsStartAndWaiting prevents a missing input from becoming
// a container restart loop followed by a readiness timeout.
func TestPreflightStopsStartAndWaiting(t *testing.T) {
	rn := NewRunner("task")
	want := errors.New("missing TOKENS_ENCRYPTION_KEY")
	h := Hooks{
		Preflight: func(_ context.Context, dir string, names []string) error {
			if dir != "plugins" || len(names) != 1 || names[0] != "tokens" {
				t.Fatalf("preflight inputs: %q %v", dir, names)
			}
			return &StepError{Plugin: "tokens", Err: want}
		},
		Start: func(context.Context) error { t.Fatal("started despite missing env"); return nil },
		HealthURL: func(Member) string {
			t.Fatal("polled despite missing env")
			return ""
		},
	}
	Run(context.Background(), rn, h, "plugins", []string{"tokens"}, false)
	f := rn.Failure()
	if f == nil || f.Step != StepStart || f.Plugin != "tokens" || !errors.Is(f.Err, want) {
		t.Fatalf("failure = %+v", f)
	}
	for _, step := range rn.Report().Steps {
		if step.Name == StepReadiness && (step.Result != ResultSkipped || !strings.Contains(step.Detail, "failed")) {
			t.Fatalf("readiness = %+v", step)
		}
	}
}
