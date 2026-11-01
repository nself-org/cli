package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEnvFilesPrecedenceAndRedaction(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	if err := os.WriteFile(first, []byte("INPUT=first\nKEEP=retained\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("INPUT=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT", "shell-value")
	values, err := ReadEnvFiles([]string{first, second})
	if err != nil || values["INPUT"] != "" || values["KEEP"] != "retained" || os.Getenv("INPUT") != "shell-value" {
		t.Fatalf("merge = %v, %v", values, err)
	}
	if err := os.WriteFile(second, []byte("BAD='private-fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ReadEnvFiles([]string{first, second})
	if err == nil || strings.Contains(err.Error(), "private-fixture") {
		t.Fatalf("unsafe syntax error: %v", err)
	}
}
