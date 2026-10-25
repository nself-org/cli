package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommandsTestsShareOneNselfBuild: the cmd/commands tests compile and link
// ./cmd/nself once per test process (pilotBinary). Each extra build cost about
// half a minute on the macOS Intel runner and pushed the package toward its
// 10m go test timeout (D-0735). A test that needs the binary calls
// pilotBinary(t). TestSurfaceIdentity re-executes itself in a fresh process, so
// it cannot share the build and is the one allowed exception.
func TestCommandsTestsShareOneNselfBuild(t *testing.T) {
	allowed := map[string]bool{
		"pilot_json_test.go":       true, // pilotBinary itself
		"surface_identity_test.go": true, // runs in a fresh process
	}
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "cmd/commands/*_test.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no cmd/commands tests found: %v", err)
	}
	for _, f := range files {
		if allowed[filepath.Base(f)] {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, `"go", "build"`) && strings.Contains(line, "cmd/nself") {
				t.Errorf("%s:%d builds ./cmd/nself itself; call pilotBinary(t) to share the one build", filepath.Base(f), i+1)
			}
		}
	}
}
