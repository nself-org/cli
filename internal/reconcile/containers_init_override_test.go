package reconcile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestD0609InitContainerLabelOverrides(t *testing.T) {
	for _, labels := range []string{"{nself.type: service}", "[nself.type=service]"} {
		t.Run(labels, func(t *testing.T) {
			dir := t.TempDir()
			files := []string{filepath.Join(dir, "base.yml"), filepath.Join(dir, "override.yml")}
			base := "services:\n  changed:\n    labels: {nself.type: init-container}\n  retained:\n    labels: [nself.type=init-container]\n"
			override := "services:\n  changed:\n    labels: " + labels + "\n  retained:\n    labels: {other: value}\n"
			for i, body := range []string{base, override} {
				if err := os.WriteFile(files[i], []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := initContainerServices(files)
			if err != nil || !reflect.DeepEqual(got, map[string]bool{"retained": true}) {
				t.Errorf("merged init labels = %v, %v; want retained only", got, err)
			}
		})
	}
}
