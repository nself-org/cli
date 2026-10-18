package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOperatorHintsDoNotNameOperatorPrivatePath guards D-0938: product text
// that advises a public-repo user must not point at the operator-private
// ~/.claude/vault.env path.
func TestOperatorHintsDoNotNameOperatorPrivatePath(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "")
	res := checkLicenseVaultPath()
	if strings.Contains(res.FixCmd, ".claude/vault") {
		t.Errorf("FixCmd names the operator-private vault path: %q", res.FixCmd)
	}

	script := filepath.Join("..", "..", "scripts", "ops", "credential-rotation-check.sh")
	b, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read %s: %v", script, err)
	}
	if strings.Contains(string(b), ".claude/vault") {
		t.Errorf("%s names the operator-private vault path", script)
	}
}
