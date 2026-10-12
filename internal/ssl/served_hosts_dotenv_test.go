package ssl

// served_hosts_dotenv_test.go — D-0444/D-0558: the fronting stack's env file is
// read with the safe reader; a missing file is still empty, a malformed one is
// reported by name and line only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEnvFileDotenv(t *testing.T) {
	const secret = "fake-planted-s3cret-0444"
	dir := t.TempDir()
	if m, err := readEnvFile(filepath.Join(dir, "absent")); err != nil || len(m) != 0 {
		t.Fatalf("missing file must be empty, got %v %v", m, err)
	}
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("GOOD=1\nT=\""+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readEnvFile(path)
	if err == nil || strings.Contains(err.Error(), secret) || err.Error() != "invalid dotenv syntax in "+path+" (line 2)" {
		t.Fatalf("got %v", err)
	}
}
