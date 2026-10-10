package repoqa

// bindings_doc_test.go — the config bindings table in the wiki stays current
// (P7-SURF-06).
//
// Purpose: .github/wiki/Configuration-Precedence.md carries the bindings table
//          between generated markers; it must equal bindings.RenderMarkdown(),
//          so the declaration (internal/config/bindings) and its documentation
//          cannot drift (Constitution 6.1).
// Constraints: `go test ./internal/repoqa -run TestBindingsDocIsCurrent -update`
//          rewrites the region in place. The -update flag is the one the
//          ratchet tests share (ratchet_test.go).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config/bindings"
)

func TestBindingsDocIsCurrent(t *testing.T) {
	b, err := bindings.Load()
	if err != nil {
		t.Fatalf("bindings.Load: %v", err)
	}
	want := b.RenderMarkdown()
	if !strings.Contains(want, "`--domain`") {
		t.Fatalf("rendered bindings lack init --domain; the check would be vacuous:\n%s", want)
	}
	path := filepath.Join(repoRoot(t), ".github", "wiki", "Configuration-Precedence.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	doc := string(raw)
	start := strings.Index(doc, bindings.BeginMarker)
	end := strings.Index(doc, bindings.EndMarker)
	if start < 0 || end < start {
		t.Fatalf("%s lacks the generated markers %q ... %q", path, bindings.BeginMarker, bindings.EndMarker)
	}
	head := doc[:start+len(bindings.BeginMarker)]
	got := doc[start+len(bindings.BeginMarker) : end]
	wantRegion := "\n\n" + want + "\n"
	if got == wantRegion {
		return
	}
	if *update {
		if err := os.WriteFile(path, []byte(head+wantRegion+doc[end:]), 0o644); err != nil {
			t.Fatalf("rewrite %s: %v", path, err)
		}
		return
	}
	t.Fatalf("%s is stale against internal/config/bindings: run `go test ./internal/repoqa -run TestBindingsDocIsCurrent -update` and commit the result", path)
}
