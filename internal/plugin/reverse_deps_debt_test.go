package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReverseDependenciesFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name, manifest string
		wantErr        bool
	}{
		{"malformed", `{broken`, true},
		{"null", `null`, true},
		{"directory name", `{"name":"pretend","dependencies":["target"]}`, false},
		{"grouped dependencies", `{"name":"pretend","dependencies":{"required":["target"],"optional":["other"]}}`, false},
		{"native metadata", `{"manifest_version":2,"license":{"tier":"free"},"dependencies":["target"]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeDepManifest(t, dir, "real-dir", tc.manifest)
			got, err := ReverseDependencies(dir, "target")
			if tc.wantErr {
				if err == nil {
					t.Error("unparseable manifest ignored")
				}
			} else if err != nil || len(got) != 1 || got[0] != "real-dir" {
				t.Errorf("got %v, %v; want directory name", got, err)
			}
		})
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".nself-state"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReverseDependencies(dir, "target"); err != nil {
		t.Fatal("state directory is not a plugin")
	}
}
