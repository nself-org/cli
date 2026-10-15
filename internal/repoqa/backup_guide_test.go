package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBackupGuideRetentionAndStrictRestore keeps the operator-facing guide
// explicit about prune holds, releasing them, and update-stack restore failures.
// Inputs: the backup guide. Outputs: missing guidance failures.
// Constraints: whitespace changes do not affect the check (D-0726).
func TestBackupGuideRetentionAndStrictRestore(t *testing.T) {
	path := filepath.Join(repoRoot(t), ".github", "wiki", "Guide-Backup-Restore.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.Join(strings.Fields(string(raw)), " ")
	for _, guidance := range []string{
		"`nself backup prune` never deletes backups tagged `keep` or `keep-*`",
		"`keep-stack-<run_id>-<nonce>`",
		"rename the backup file",
		"normal retention applies",
		"`nself update stack` restores strictly",
		"any unexpected `pg_restore` `error:` line fails the restore",
		"even in v1.4 compatibility mode",
	} {
		if !strings.Contains(doc, guidance) {
			t.Errorf("backup guide lacks guidance: %s", guidance)
		}
	}
}
