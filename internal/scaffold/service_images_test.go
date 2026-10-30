package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
)

func TestServiceDockerfilesUseImageLock(t *testing.T) {
	for _, mode := range []string{"legacy", "lock"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("IMAGE_PINNING", mode)
			for lang, names := range map[string][]string{
				"go": {"golang", "distroless-static"}, "node": {"node"},
				"python": {"python"}, "static": {"nginx"},
				"rust": {"rust", "alpine"}, "other": {"alpine"},
			} {
				t.Run(lang, func(t *testing.T) {
					dir := t.TempDir()
					if _, err := writeScaffold(dir, "example", lang, 3000, compose.ImageRef); err != nil {
						t.Fatal(err)
					}
					raw, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
					if err != nil {
						t.Fatal(err)
					}
					var got []string
					for _, line := range strings.Split(string(raw), "\n") {
						if strings.HasPrefix(line, "FROM ") {
							got = append(got, strings.Fields(line)[1])
						}
					}
					if len(got) != len(names) {
						t.Fatalf("got %d stages, want %d", len(got), len(names))
					}
					for i, name := range names {
						want := compose.ImageRef(name, "")
						if want == "" || got[i] != want {
							t.Errorf("stage %d: got %q, want lock entry %s (%q)", i, got[i], name, want)
						}
					}
				})
			}
		})
	}
}

func TestServiceRequiresImageResolver(t *testing.T) {
	dir := t.TempDir()
	_, err := Run(Options{Name: "example", Lang: "go", ProjectDir: dir})
	if err == nil || !strings.Contains(err.Error(), "image resolver is required") {
		t.Fatalf("Run without resolver: got %v, want required resolver error", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "services")); !os.IsNotExist(err) {
		t.Fatalf("Run without resolver created service files: %v", err)
	}
	if _, err := writeScaffold(dir, "example", "go", 3000, nil); err == nil {
		t.Fatal("writeScaffold without resolver must return an error")
	}
}
