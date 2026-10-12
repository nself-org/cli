package plugin

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestPrintBuildHint: only v1.5 with a caller-reconciles context is silent.
func TestPrintBuildHint(t *testing.T) {
	cases := []struct {
		name   string
		v15    string
		marked bool
		silent bool
	}{
		{"v1.4 unmarked", "0", false, false},
		{"v1.4 marked", "0", true, false},
		{"v1.5 unmarked", "1", false, false},
		{"v1.5 marked", "1", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("NSELF_V15", c.v15)
			var buf bytes.Buffer
			prev := hintOut
			hintOut = &buf
			t.Cleanup(func() { hintOut = prev })

			ctx := context.Background()
			if c.marked {
				ctx = WithCallerReconciles(ctx)
			}
			printBuildHint(ctx, "demo")

			got := buf.String()
			if c.silent && got != "" {
				t.Fatalf("expected no hint, got %q", got)
			}
			if !c.silent && !strings.Contains(got, "Run 'nself build' to include demo in your stack.") {
				t.Fatalf("expected the hint, got %q", got)
			}
		})
	}
}
