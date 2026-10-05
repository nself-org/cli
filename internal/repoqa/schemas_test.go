package repoqa

import (
	"os"
	"os/exec"
	"testing"
)

// TestSchemasAreCurrent fails when schemas/*.json no longer match what
// tools/schemagen generates from the Go types (P7-REG-08), the same drift
// guard TestParityMatrixIsCurrent applies to the parity matrix.
func TestSchemasAreCurrent(t *testing.T) {
	root := repoRoot(t)

	gen := exec.Command("go", "run", "-mod=vendor", "./tools/schemagen", "-check")
	gen.Dir = root
	gen.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := gen.CombinedOutput()
	if err != nil {
		t.Fatalf("schemas are stale; run `make schemas` and commit the result:\n%s", out)
	}
}
