package repoqa

// Purpose: Keep the PostgreSQL credential lifecycle and upgrade guidance visible.
// Inputs: The committed PostgreSQL wiki page and changelog (D-0706, D-0506).
// Outputs: Failures when docs require an explicit password or omit persistence,
// precedence, or the manual pinning step for credentials from older builds.
// Constraints: Read-only; checks documentation without loading project secrets.
// SPORT: cli/internal/build/orchestrator_secrets.go and internal/config/cascade.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresPasswordDocumentation(t *testing.T) {
	readWiki := func(name string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "wiki", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	doc := readWiki("Config-Postgres.md")
	for _, stale := range []string{"will never generate", "must be set explicitly"} {
		if strings.Contains(doc, stale) {
			t.Errorf("PostgreSQL docs still claim %q", stale)
		}
	}
	var passwordRow string
	for line := range strings.SplitSeq(doc, "\n") {
		if strings.HasPrefix(line, "| `POSTGRES_PASSWORD` |") {
			passwordRow = line
			break
		}
	}
	cells := strings.Split(passwordRow, "|")
	if len(cells) < 6 || strings.TrimSpace(cells[3]) != "No" ||
		!strings.Contains(strings.ToLower(cells[2]), "generated") {
		t.Errorf("POSTGRES_PASSWORD must be optional with a generated default: %q", passwordRow)
	}
	for _, topic := range []string{
		"`nself build`", "`.env.secrets`", "0600", "`.env.<env>`", "`.env.local`",
		"process export", "any env file", "`.nself/compose.env`", "not automatically persisted",
	} {
		if !strings.Contains(doc, topic) {
			t.Errorf("PostgreSQL credential guidance omits %q", topic)
		}
	}
	changelog := readWiki("Changelog.md")
	var upgradeNote string
	for paragraph := range strings.SplitSeq(changelog, "\n\n") {
		if strings.Contains(paragraph, "D-0506") {
			upgradeNote = paragraph
			break
		}
	}
	for _, topic := range []string{
		"#710", "POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", ".env.secrets",
		".nself/compose.env", "not automatically persisted", "pin",
	} {
		if !strings.Contains(upgradeNote, topic) {
			t.Errorf("D-0506 upgrade note omits %q", topic)
		}
	}
}
