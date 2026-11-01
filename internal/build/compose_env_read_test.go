package build

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestReadComposeEnvOptionalDotenv distinguishes an absent optional default
// file from an existing file that Compose cannot parse or read.
func TestReadComposeEnvOptionalDotenv(t *testing.T) {
	root := t.TempDir()
	values, err := ReadComposeEnv(root)
	if err != nil || len(values) != 0 {
		t.Fatalf("absent env = %v, %v", values, err)
	}
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("BROKEN='unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ReadComposeEnv(root)
	var syntax *config.EnvSyntaxError
	if !errors.As(err, &syntax) {
		t.Fatalf("expected syntax error for existing file, got %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadComposeEnv(root); err == nil {
		t.Fatal("unreadable dotenv directory accepted")
	}
}
