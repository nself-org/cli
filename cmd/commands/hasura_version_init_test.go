package commands

// Purpose: distinguish mode defaults from explicit Hasura versions (D-0720).
// Inputs: real init env files and the service upgrade handler in both modes.
// Outputs: rendered images, including overrides of an existing project pin.
// Constraints: no Docker or network; use the production config loader.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/setup"
	"github.com/spf13/cobra"
)

func TestHasuraVersionExplicitAfterInit(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		for _, demo := range []bool{false, true} {
			for _, mode := range []string{"legacy", "lock"} {
				t.Run(fmt.Sprintf("demo=%v/%s", demo, mode), func(t *testing.T) {
					blankImageVars(t)
					t.Setenv("IMAGE_PINNING", mode)
					dir := t.TempDir()
					t.Chdir(dir)
					if _, err := setup.Initialize(setup.Options{WorkDir: dir, Name: "hasurapin", Demo: demo, NonInteractive: true, Quiet: true}); err != nil {
						t.Fatal(err)
					}
					raw, err := os.ReadFile(filepath.Join(dir, ".env"))
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(raw), "\nHASURA_VERSION=\n") {
						t.Error("init must leave HASURA_VERSION blank for the mode default")
					}
					defer compose.SetProjectPins(nil)()
					wantDefault := compose.ImageRef(compose.HasuraEntry(), "")
					if got := renderStackIn(t, dir, nil)["hasura"]; got != wantDefault {
						t.Errorf("init image = %s, want %s", got, wantDefault)
					}
					// A newer project pin must yield to an explicit downgrade, too.
					for _, pinned := range []bool{false, true} {
						if pinned {
							compose.SetProjectPins(map[string]string{"hasura": compose.ImageRef("hasura", "")})
						}
						cmd := &cobra.Command{}
						cmd.Flags().String("env", "", "")
						if err := runServiceUpgrade(cmd, []string{"hasura", "v2.44.0"}); err != nil {
							t.Fatal(err)
						}
						want := compose.ImageRef("hasura-v14", "")
						if got := renderStackIn(t, dir, nil)["hasura"]; got != want {
							t.Errorf("upgrade v2.44.0 (pinned=%v) image = %s, want %s", pinned, got, want)
						}
					}
				})
			}
		}
	})
}
