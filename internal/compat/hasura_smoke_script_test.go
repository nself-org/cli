package compat

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestHasuraSmokeStartsSkipMetadataReapply guards D-1152: after the baseline
// start, every `nself start` in scripts/ci/hasura-upgrade-smoke.sh must pass
// --skip-db-init. A plain start re-applies the saved metadata
// (hasura.ApplyIfPresent), which would repair metadata an upgrade lost and let
// the preservation checks pass. The script itself needs Docker, so this test
// reads the script text only.
func TestHasuraSmokeStartsSkipMetadataReapply(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "ci", "hasura-upgrade-smoke.sh")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	startCmd := regexp.MustCompile(`^\s*nsf start(\s|$)`)
	baseline := regexp.MustCompile(`^# ---- step 4`)
	inBuildStart, afterSwitch, checked := false, false, 0
	for i, line := range lines {
		if baseline.MatchString(line) {
			afterSwitch = true
		}
		if strings.HasPrefix(line, "build_start()") {
			inBuildStart = true
		} else if inBuildStart && line == "}" {
			inBuildStart = false
		}
		// build_start is only used after the switch (restore), so it counts
		// whatever its position in the file.
		if !startCmd.MatchString(line) || !(afterSwitch || inBuildStart) {
			continue
		}
		checked++
		if !strings.Contains(line, "--skip-db-init") {
			t.Errorf("%s:%d: %q must pass --skip-db-init so the start does not re-apply metadata before the preservation checks", path, i+1, strings.TrimSpace(line))
		}
	}
	if checked < 3 {
		t.Fatalf("found %d guarded starts in %s, want at least 3 (switch, pin-back, build_start)", checked, path)
	}
}
