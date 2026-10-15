package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigValidateDocumentsDollarWarning keeps the command-page PROSE block
// explicit about the warning's trigger, privacy, exception and exit status.
func TestConfigValidateDocumentsDollarWarning(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "wiki", "cmd-config.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, prose, ok := strings.Cut(string(raw), "<!-- BEGIN PROSE:description -->")
	if !ok {
		t.Fatal("config command page lacks description PROSE")
	}
	prose, _, ok = strings.Cut(prose, "<!-- END PROSE:description -->")
	if !ok {
		t.Fatal("config command page lacks description PROSE end")
	}
	for _, want := range []string{
		"warns",
		"unquoted or double-quoted",
		"`$` the loader would expand",
		"file:line and key only, never the value",
		"`${NAME}` references are not flagged",
		"warnings do not change the exit code",
	} {
		if !strings.Contains(prose, want) {
			t.Errorf("config validate description missing %q", want)
		}
	}
}
