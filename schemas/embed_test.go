package schemas

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedSchemasMatchCommittedFiles(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("no schemas embedded")
	}
	for _, name := range names {
		got, ok := Lookup(name)
		want, err := os.ReadFile(filepath.FromSlash(name))
		if err != nil || !ok || !bytes.Equal(got, want) {
			t.Errorf("%s: embedded bytes differ from committed file: %v", name, err)
		}
	}
	if _, ok := Lookup("../go.mod"); ok {
		t.Fatal("path traversal accepted")
	}
}
