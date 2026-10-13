package signature_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSignaturetestImportedOnlyByTests: signaturetest can swap the trust root,
// so no non-test file in the module may import it.
func TestSignaturetestImportedOnlyByTests(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..") // cli module root
	needle := "internal/plugin/verify/signature/signaturetest"
	scanned := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", ".git", ".claude", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		if strings.Contains(filepath.ToSlash(path), "verify/signature/signaturetest/") {
			return nil // the package itself
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), needle) {
			t.Errorf("%s imports signaturetest outside a test file", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 500 {
		t.Fatalf("scanned only %d files; the walk is wrong", scanned)
	}
}
