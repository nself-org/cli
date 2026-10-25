package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/lock"
)

// Purpose: run the wiki's "Without a saved lock" Hasura rollback commands against
// a lockless project upgraded to v2.51.0, whose generated docker-compose.yml is
// the pin source for the first lock.
// Inputs: the wiki command example and a v1.4 project with a v2.51.0 compose file.
// Outputs: failures when the documented build leaves Hasura on v2.51.0.
// Constraints: no shell, network or Docker; only mv, unset and nself build are
// simulated. SPORT: D-1153.
func TestHasuraRollbackWithoutLockRecoversComposePin(t *testing.T) {
	data, err := os.ReadFile("../../.github/wiki/Image-Lock.md")
	if err != nil {
		t.Fatal(err)
	}
	_, doc, ok := strings.Cut(string(data), "#### Without a saved lock")
	if !ok {
		t.Fatal("missing rollback route")
	}
	doc, _, _ = strings.Cut(doc, "\n## ")
	_, commands, ok := strings.Cut(doc, "```bash\n")
	if !ok {
		t.Fatal("missing executable rollback example")
	}
	commands, _, _ = strings.Cut(commands, "```")

	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, false)
	upgraded, _ := compose.LockedRef("hasura")
	withExistingCompose(t, f, "services:\n  hasura:\n    image: "+upgraded.String()+"\n")
	t.Setenv("HASURA_VERSION", "v2.51.0")

	built := false
	for _, line := range strings.Split(commands, "\n") {
		fields := strings.Fields(line)
		switch {
		case line == "unset HASURA_VERSION":
			t.Setenv("HASURA_VERSION", "")
		case len(fields) == 3 && fields[0] == "mv":
			if err := os.Rename(filepath.Join(f.workdir, fields[1]), filepath.Join(f.workdir, fields[2])); err != nil {
				t.Fatalf("%q: %v", line, err)
			}
		case strings.Contains(line, "nself build"):
			t.Setenv("IMAGE_PINNING", "lock")
			opts := BuildOptions{Force: true, WriteLock: strings.Contains(line, "--write-lock")}
			if _, err := Build(f.workdir, opts); err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			built = true
		}
	}
	if !built {
		t.Fatal("rollback example has no build command")
	}
	want, _ := compose.LockedRef("hasura-v14")
	if got := composeImagesOf(t, f)["hasura"]; got != want.String() {
		t.Errorf("the documented rollback renders %q, want %q", got, want.String())
	}
	if got := readLock(t, f).Images["hasura"]; got != want.String() {
		t.Errorf("the documented rollback pins %q in %s, want %q", got, lock.FileName, want.String())
	}
}
