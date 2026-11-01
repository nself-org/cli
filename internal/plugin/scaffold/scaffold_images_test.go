package scaffold

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
)

func TestPluginDockerfilesUseImageLock(t *testing.T) {
	for _, mode := range []string{"legacy", "lock"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("IMAGE_PINNING", mode)
			for lang, names := range map[string][]string{
				"go": {"golang", "distroless-static"}, "node": {"node", "node"},
				"static": {"nginx"}, "rust": {"rust", "alpine"},
			} {
				t.Run(lang, func(t *testing.T) {
					body, err := buildDockerfile(Params{Language: lang, Name: "example", Port: 3000})
					if err != nil {
						t.Fatal(err)
					}
					var got []string
					for _, line := range strings.Split(body, "\n") {
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

func TestUnknownTemplateImageFails(t *testing.T) {
	if _, err := render(`FROM {{image "missing-scaffold-image"}}`, Params{}); err == nil {
		t.Fatal("missing lock entry must fail template rendering")
	}
}
