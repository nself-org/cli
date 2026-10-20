package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasuraUpgradeStartsPreserveDatabase(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "ci", "hasura-upgrade-smoke.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, section := range []string{"# ---- step 4:", "build_start()"} {
		t.Run(section, func(t *testing.T) {
			_, tail, ok := strings.Cut(script, section)
			if !ok {
				t.Fatal("missing smoke section")
			}
			_, tail, ok = strings.Cut(tail, "nsf start")
			if !ok {
				t.Fatal("missing start")
			}
			line, _, _ := strings.Cut(tail, "\n")
			if !strings.Contains(line, "--skip-db-init") {
				t.Fatal("preservation check can reapply metadata on start")
			}
		})
	}
}
