package repoqa

// Purpose: keep the generated @nself/cli-api client in step with .github/openapi.json.
// Inputs: committed OpenAPI and sdk/ts-cli-api/src/generated. Outputs: a drift verdict.
// Constraints: vendor-only; no network.

import (
	"os/exec"
	"testing"
)

func TestSDKTSIsCurrent(t *testing.T) {
	gen := exec.Command("go", "run", "-mod=vendor", "./tools/sdkgen", "-lang", "ts", "-check")
	gen.Dir = repoRoot(t)
	gen.Env = append(gen.Environ(), "CGO_ENABLED=0")
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generated TypeScript client drift: %v\n%s", err, out)
	}
}
