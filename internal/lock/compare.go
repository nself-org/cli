package lock

// compare.go — Compare, the frozen check of `nself start --frozen`.
//
// Purpose: say, deterministically, how the stack on disk differs from the lock.
// Inputs: the locked Lock and the Lock computed from the current state.
// Outputs: Mismatch values sorted by kind then name; none means equal.
// Constraints: pure. An empty cli or sha256 on either side means "unknown" and
// is not compared. Digest-pinned and tag-only entries compare as one map of
// service to reference.

import (
	"fmt"
	"sort"
)

// Mismatch kinds.
const (
	KindCLI    = "cli"
	KindImage  = "image"
	KindPlugin = "plugin"
	KindBundle = "bundle"
)

// Absent is the value shown for a side that has no entry.
const Absent = "<absent>"

// Mismatch is one difference between the lock and the current state.
type Mismatch struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Locked string `json:"locked"`
	Actual string `json:"actual"`
}

// String renders the mismatch for a terminal.
func (m Mismatch) String() string {
	if m.Name == "" {
		return fmt.Sprintf("%s: lock has %s, stack has %s", m.Kind, m.Locked, m.Actual)
	}
	return fmt.Sprintf("%s %s: lock has %s, stack has %s", m.Kind, m.Name, m.Locked, m.Actual)
}

// Compare lists how actual differs from locked.
func Compare(locked, actual Lock) []Mismatch {
	var out []Mismatch
	if locked.CLI != "" && actual.CLI != "" && locked.CLI != actual.CLI {
		out = append(out, Mismatch{KindCLI, "", locked.CLI, actual.CLI})
	}
	lp, ap := locked.Pins(), actual.Pins()
	for _, k := range unionKeys(lp, ap) {
		if lp[k] != ap[k] {
			out = append(out, Mismatch{KindImage, k, orAbsent(lp, k), orAbsent(ap, k)})
		}
	}
	names := map[string]bool{}
	for k := range locked.Plugins {
		names[k] = true
	}
	for k := range actual.Plugins {
		names[k] = true
	}
	for _, k := range sortedSet(names) {
		lv, lok := locked.Plugins[k]
		av, aok := actual.Plugins[k]
		switch {
		case !lok:
			out = append(out, Mismatch{KindPlugin, k, Absent, av.Version})
		case !aok:
			out = append(out, Mismatch{KindPlugin, k, lv.Version, Absent})
		case lv.Version != av.Version:
			out = append(out, Mismatch{KindPlugin, k, lv.Version, av.Version})
		case lv.SHA256 != "" && av.SHA256 != "" && lv.SHA256 != av.SHA256:
			out = append(out, Mismatch{KindPlugin, k + " sha256", lv.SHA256, av.SHA256})
		}
	}
	lb, ab := toSet(locked.Bundles), toSet(actual.Bundles)
	for _, k := range sortedSet(mergeSets(lb, ab)) {
		if lb[k] != ab[k] {
			out = append(out, Mismatch{KindBundle, k, present(lb[k]), present(ab[k])})
		}
	}
	return out
}

func present(b bool) string {
	if b {
		return "installed"
	}
	return Absent
}

func orAbsent(m map[string]string, k string) string {
	if v, ok := m[k]; ok {
		return v
	}
	return Absent
}

func unionKeys(a, b map[string]string) []string {
	s := map[string]bool{}
	for k := range a {
		s[k] = true
	}
	for k := range b {
		s[k] = true
	}
	return sortedSet(s)
}

func toSet(v []string) map[string]bool {
	s := map[string]bool{}
	for _, x := range v {
		s[x] = true
	}
	return s
}

func mergeSets(a, b map[string]bool) map[string]bool {
	s := map[string]bool{}
	for k := range a {
		s[k] = true
	}
	for k := range b {
		s[k] = true
	}
	return s
}

func sortedSet(s map[string]bool) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
