package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestComposeStorageDocModes keeps both compose ordering sections explicit about
// compatibility modes and directs existing MinIO users to the migration guide.
func TestComposeStorageDocModes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/wiki/Compose-Generation.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for _, heading := range []string{"Multi-File Compose Merge", "Service Generation Order"} {
		t.Run(heading, func(t *testing.T) {
			_, section, ok := strings.Cut(doc, "## "+heading+"\n")
			if !ok {
				t.Fatalf("missing section %q", heading)
			}
			section, _, _ = strings.Cut(section, "\n## ")
			for _, want := range []string{"v1.4", "`minio`", "init", "v1.5", "`seaweedfs`", "`seaweedfs-init`"} {
				if !strings.Contains(section, want) {
					t.Errorf("storage ordering omits %q", want)
				}
			}
		})
	}
	for _, want := range []string{"OBJECT_STORAGE_ENABLED=true", "NSELF_V15=1", "nself add minio", "E138", "[[Object-Storage]]"} {
		if !strings.Contains(doc, want) {
			t.Errorf("storage guidance omits %q", want)
		}
	}
}
