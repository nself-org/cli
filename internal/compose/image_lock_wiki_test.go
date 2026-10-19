package compose

// image_lock_wiki_test.go — the Hasura rollback steps in the Image-Lock wiki
// page stay true to how the image is rendered (D-1153).
//
// Purpose: HASURA_VERSION=v2.51.0 is an override that drops a lock pin of
// v2.44.0, and in v1.5 a blank or v2.44.0 value renders v2.51.0 when no pin
// exists. The "Roll back" steps must therefore say to clear the override, and
// must not promise that removing it reaches v2.44.0 on a project without a lock.
// Inputs: .github/wiki/Image-Lock.md, the Hasura "Roll back" section.
// Outputs: test failures naming the missing or false statement.
// Constraints: no network, no Docker.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hasuraRollbackSection returns the text of the Hasura "Roll back" section.
func hasuraRollbackSection(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "wiki", "Image-Lock.md"))
	if err != nil {
		t.Fatalf("read Image-Lock.md: %v", err)
	}
	doc := string(raw)
	hasura := strings.Index(doc, "## Hasura:")
	if hasura < 0 {
		t.Fatal("Image-Lock.md has no Hasura section")
	}
	rest := doc[hasura:]
	start := strings.Index(rest, "### Roll back")
	if start < 0 {
		t.Fatal("Hasura section has no Roll back subsection")
	}
	rest = rest[start:]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func TestHasuraRollbackDocClearsOverride(t *testing.T) {
	sec := hasuraRollbackSection(t)
	var step3 string
	for _, line := range strings.Split(sec, "\n") {
		if strings.HasPrefix(line, "3. ") {
			step3 = line
		}
	}
	if step3 == "" {
		t.Fatal("Roll back has no step 3")
	}
	for _, want := range []string{"HASURA_VERSION=v2.51.0", "override", "nself.lock.yaml", "v1.4 binary"} {
		if !strings.Contains(step3, want) {
			t.Errorf("Roll back step 3 does not mention %q:\n%s", want, step3)
		}
	}
	// With the override gone and no pin, v1.5 renders v2.51.0, so the old
	// promise that removing the variable alone goes back is false.
	if strings.Contains(sec, "can go back by removing `HASURA_VERSION=v2.51.0`") {
		t.Error("Roll back still says a project without a lock can go back by removing HASURA_VERSION")
	}
}
