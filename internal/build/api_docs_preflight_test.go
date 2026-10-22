package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIDocsPathPreflightNoWrites(t *testing.T) {
	for _, path := range []string{"/tmp/bad path", "relative/docs"} {
		t.Run(path, func(t *testing.T) {
			f := freshProject(t, "dev-minimal")
			envPath := filepath.Join(f.workdir, ".env")
			raw, err := os.ReadFile(envPath)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, envPath, string(raw)+"\nAPI_DOCS_ENABLED=true\nAPI_DOCS_PATH="+path+"\n", 0644)
			before := treeEntries(t, f.workdir)
			_, err = Build(f.workdir, BuildOptions{Force: true})
			if err == nil || !strings.Contains(err.Error(), "API_DOCS_PATH") {
				t.Fatalf("want path error, got %v", err)
			}
			if treeEntries(t, f.workdir) != before {
				t.Fatal("invalid API_DOCS_PATH changed project files")
			}
		})
	}
}
