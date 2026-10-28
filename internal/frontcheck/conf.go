// Copyright © 2024 nself-org. All rights reserved.

package frontcheck

import (
	"regexp"
	"sort"
	"strings"
)

// ConfMax caps how much of a host conf is read.
const ConfMax = 262144

var serverNameRe = regexp.MustCompile(`\bserver_name[ \t]+([^;]+);`)

// HasBuildMarker is the strict marker rule on conf text.
func HasBuildMarker(conf, marker string) bool {
	for i, line := range strings.Split(conf, "\n") {
		if i >= 5 {
			break
		}
		if strings.HasPrefix(line, marker) {
			return true
		}
	}
	return false
}

// ServerNameSet is the sorted, unique server_name values of the conf (comments dropped).
func ServerNameSet(conf string) []string {
	seen := map[string]bool{}
	for _, line := range strings.Split(conf, "\n") {
		line, _, _ = strings.Cut(line, "#")
		for _, m := range serverNameRe.FindAllStringSubmatch(line, -1) {
			for _, n := range strings.Fields(m[1]) {
				seen[n] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
