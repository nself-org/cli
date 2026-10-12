package promote

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadComposeImages(t *testing.T) {
	dir := t.TempDir()
	base, override := filepath.Join(dir, "base.yml"), filepath.Join(dir, "override.yml")
	if err := os.WriteFile(base, []byte("services:\n  api:\n    image: example.invalid/api:old\n  worker:\n    build: .\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(override, []byte("services:\n  api:\n    image: example.invalid/api:new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	images, err := ReadComposeImages([]string{base, override})
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 || images[0].Service != "api" || images[0].Ref != "example.invalid/api:new" || images[0].Build || images[1].Service != "worker" || !images[1].Build {
		t.Fatalf("ReadComposeImages() = %#v", images)
	}
}
