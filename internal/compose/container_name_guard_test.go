package compose

// container_name_guard_test.go — D-0130 guard: a service's container name is
// built by config.Config.ServiceContainerName alone. A builder that formats
// `<PROJECT_NAME>_<svc>` itself makes a naming change a dozen edits again.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// handRolledName matches a ContainerName field set from Sprintf("%s_<svc>", ...
// ProjectName) or ProjectName + "_<svc>". Network and volume names share the
// prefix but are not container names, so only ContainerName lines count.
var handRolledName = regexp.MustCompile(`ContainerName\b.*("%s_[^"]*"\s*,[^)]*ProjectName|ProjectName\s*\+\s*"_)`)

func TestContainerNamesComeFromOneHelper(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no compose sources found: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if handRolledName.MatchString(line) {
				t.Errorf("%s:%d builds a container name by hand, use cfg.ServiceContainerName: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// TestHandRolledNamePatternControl proves the guard matches both shapes it
// exists to stop.
func TestHandRolledNamePatternControl(t *testing.T) {
	for _, bad := range []string{
		`ContainerName: fmt.Sprintf("%s_redis", g.cfg.ProjectName),`,
		`ContainerName: cfg.ProjectName + "_postgres",`,
	} {
		if !handRolledName.MatchString(bad) {
			t.Errorf("pattern misses %q", bad)
		}
	}
	if handRolledName.MatchString(`ContainerName: g.cfg.ServiceContainerName("redis"),`) {
		t.Error("pattern flags the helper call")
	}
}
