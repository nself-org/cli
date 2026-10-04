package cmdregistry

import "github.com/nself-org/cli/internal/canon"

// computeCounts derives Counts from a command list. top_level counts
// non-hidden depth-1 commands excluding help (the inventory and surface-budget
// rule). core_missing lists verbs with no depth-1 command, in verb order; when
// missing is non-nil it is used as is (a subtree keeps the registry-wide list).
func computeCounts(verbs []string, cmds []Command, rootPath string, missing []string) Counts {
	n := Counts{Commands: len(cmds)}
	top := map[string]bool{}
	for _, c := range cmds {
		if c.Parent == rootPath {
			top[c.Name] = true
			if !c.Hidden && c.Name != "help" {
				n.TopLevel++
			}
		}
		switch c.Canon {
		case canon.CanonCore:
			n.Core++
		case canon.CanonPending:
			n.Pending++
		case canon.CanonShim:
			n.DeprecatedShims++
		}
		switch c.JSON {
		case canon.JSONEnvelope:
			n.JSONEnvelope++
		case canon.JSONLegacy:
			n.JSONLegacy++
		}
	}
	if missing != nil {
		n.CoreMissing = missing
		return n
	}
	n.CoreMissing = []string{}
	for _, v := range verbs {
		if !top[v] {
			n.CoreMissing = append(n.CoreMissing, v)
		}
	}
	return n
}
