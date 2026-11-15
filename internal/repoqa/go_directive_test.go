package repoqa

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// minimumGoRelease is the toolchain floor for GO-2026-6859 (net/http).
const minimumGoRelease = "1.26.11"

func goReleaseBelowFloor(version string) bool {
	var got [3]int
	var minimum [3]int
	for i, part := range strings.Split(minimumGoRelease, ".") {
		minimum[i], _ = strconv.Atoi(part)
	}
	for i, part := range strings.Split(version, ".") {
		if i >= len(got) {
			return true
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return true
		}
		got[i] = n
	}
	for i, n := range got {
		if n != minimum[i] {
			return n < minimum[i]
		}
	}
	return false
}

func TestGoDirectiveAtLeastPatchedRelease(t *testing.T) {
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^go ([0-9]+\.[0-9]+\.[0-9]+)\s*$`).FindSubmatch(data)
	if m == nil || goReleaseBelowFloor(string(m[1])) {
		t.Fatalf("go.mod needs a full Go release at least %v (GO-2026-6859)", minimumGoRelease)
	}
	for version, below := range map[string]bool{"1.26.10": true, "1.26.11": false, "1.27.0": false} {
		if goReleaseBelowFloor(version) != below {
			t.Errorf("floor comparison for %s", version)
		}
	}
}

func TestWorkflowsDoNotPinOlderGo(t *testing.T) {
	fullVersion := regexp.MustCompile(`^\s*go-version:\s*['"]?([0-9]+\.[0-9]+\.[0-9]+)['"]?\s*(#.*)?$`)
	err := filepath.WalkDir("../../.github/workflows", func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if m := fullVersion.FindStringSubmatch(line); m != nil && goReleaseBelowFloor(m[1]) {
				t.Error(fmt.Sprintf("%s:%d: Go %s is below %v; use go-version-file: go.mod", path, i+1, m[1], minimumGoRelease))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
