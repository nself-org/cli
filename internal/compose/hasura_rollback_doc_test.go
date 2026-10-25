package compose

import (
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

// Purpose: keep the documented Hasura rollback commands on the pre-upgrade image.
// Inputs: wiki command examples, embedded images and simulated saved project pin.
// Outputs: failures when either rollback route selects the upgraded image.
// Constraints: no shell execution, network, Docker or project file writes.
// SPORT: D-1153.
func TestHasuraRollbackDocumentation(t *testing.T) {
	data, err := os.ReadFile("../../.github/wiki/Image-Lock.md")
	if err != nil {
		t.Fatal(err)
	}
	_, doc, ok := strings.Cut(string(data), "## Hasura:")
	if !ok {
		t.Fatal("missing Hasura documentation")
	}
	_, doc, _ = strings.Cut(doc, "### Roll back")
	doc, _, _ = strings.Cut(doc, "## How pins move")
	// Every supported cascade layer must be covered, including legacy order.
	for _, legacy := range []bool{false, true} {
		for _, file := range config.EnvCascadeOrder("prod", legacy) {
			if !strings.Contains(doc, "`"+file+"`") {
				t.Errorf("rollback does not cover override removal from %s", file)
			}
		}
	}
	old, _ := LockedRef("hasura-v14")
	for _, route := range []struct {
		heading string
		pins    map[string]string
	}{
		{"#### With a saved lock", map[string]string{"hasura": old.String()}},
		// nil pins model the route's state after the documented mv of
		// docker-compose.yml; internal/build covers the recovered-pin case.
		{"#### Without a saved lock", nil},
	} {
		t.Run(route.heading, func(t *testing.T) {
			defer SetProjectPins(route.pins)()
			checkHasuraRollbackCommands(t, doc, route.heading, old.String())
		})
	}
}

func checkHasuraRollbackCommands(t *testing.T, doc, heading, want string) {
	t.Helper()
	_, section, ok := strings.Cut(doc, heading)
	if !ok {
		t.Fatalf("missing rollback route %q", heading)
	}
	section, _, _ = strings.Cut(section, "\n#### ")
	_, commands, ok := strings.Cut(section, "```bash\n")
	if !ok {
		t.Fatal("missing executable rollback example")
	}
	commands, _, _ = strings.Cut(commands, "```")
	version, count := "v2.51.0", 0
	for _, line := range strings.Split(commands, "\n") {
		if line == "unset HASURA_VERSION" {
			version = ""
			continue
		}
		if !strings.Contains(line, "nself build") && !strings.Contains(line, "nself start") {
			continue
		}
		count++
		if !strings.Contains(line, "IMAGE_PINNING=lock ") {
			t.Errorf("%q must select lock image pinning", line)
		}
		if heading == "#### Without a saved lock" && strings.Contains(line, "nself build") && !strings.Contains(line, "--write-lock") {
			t.Errorf("%q must save the v1.4 rollback pin", line)
		}
		// Each command starts in the caller's v1.5 environment. An inline
		// assignment applies only to that command, so start needs it too.
		compattest.Set(t, !strings.HasPrefix(line, "NSELF_V15=0 "))
		setMode(t, PinningLock)
		t.Setenv("HASURA_VERSION", version)
		if got := hasuraImageFor(t, version); got != want {
			t.Errorf("%q with HASURA_VERSION=%q renders %q, want %q", line, version, got, want)
		}
	}
	if count != 2 {
		t.Errorf("rollback example has %d build/start commands, want both", count)
	}
}
