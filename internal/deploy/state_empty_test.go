package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDeployStateRejectsEmptyRecords(t *testing.T) {
	for _, body := range []string{"null", "{}", `{"unknown":true}`} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, DeployStateFile)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadDeployState(dir); err == nil {
				t.Fatal("empty state must not permit legacy rollback fallback")
			}
		})
	}
}
