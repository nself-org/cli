package doctor

import (
	"os"
	"strings"
	"testing"
)

// TestNoOperatorPrivatePathAdvice (D-0938): product text that only advises the
// user must not point public-repo users at an operator-private secrets path.
// check_ci_vault_sync.go is exempt: it is the one place that reads the file.
func TestNoOperatorPrivatePathAdvice(t *testing.T) {
	for _, p := range []string{
		"dogfood_checks.go",
		"../../scripts/ops/credential-rotation-check.sh",
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if strings.Contains(string(b), "vault.env") {
			t.Errorf("%s mentions the operator-private vault.env path", p)
		}
	}
}
