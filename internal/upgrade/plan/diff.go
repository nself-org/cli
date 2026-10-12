package plan

// diff.go — Diff, BuildPlan and ApplyImagePins.
//
// Purpose: compare a project Lock with the candidates the caller resolved.
// Inputs: lock.Lock and Candidates.
// Outputs: Pin rows sorted by kind (cli, image, plugin) then name.
// Constraints: a pin whose candidate equals it (an image with the same digest
// in any spelling, a plugin at the same version, the CLI at the same version)
// is not a row. A lock key with no candidate, or a candidate carrying a Reason,
// is a row with Resolvable=false.

import (
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/lock"
)

// Candidate is what a source offers for one pin.
type Candidate struct {
	// Value is the offered image reference, plugin version or CLI version.
	Value string
	// Reason is set when the source cannot offer the pin (for example it
	// answered 404); Value is then ignored.
	Reason string
	// Note is carried to the Pin of a resolvable image candidate.
	Note string
}

// Candidates is everything the sources offered. A key missing from Images or
// Plugins means the source does not know the pin.
type Candidates struct {
	// CLICurrent is the running CLI version; CLI is the latest release.
	CLICurrent string
	CLI        Candidate
	Images     map[string]Candidate
	Plugins    map[string]Candidate
}

// Reasons used for rows the sources cannot answer.
const (
	ReasonNoImage  = "not in the CLI image lock"
	ReasonNoPlugin = "not in the plugin registry"
)

// BuildPlan is Diff plus the ordered steps.
func BuildPlan(l lock.Lock, c Candidates) Plan {
	pins := Diff(l, c)
	return Plan{Pins: pins, Steps: DefaultSteps(pins)}
}

// Diff lists how the offers differ from the lock.
func Diff(l lock.Lock, c Candidates) []Pin {
	pins := []Pin{}
	if c.CLI.Reason != "" {
		pins = append(pins, Pin{Kind: KindCLI, Name: "nself", From: c.CLICurrent, Reason: c.CLI.Reason})
	} else if c.CLI.Value != "" && trimV(c.CLI.Value) != trimV(c.CLICurrent) {
		pins = append(pins, Pin{Kind: KindCLI, Name: "nself", From: c.CLICurrent, To: c.CLI.Value, Resolvable: true})
	}
	cur := l.Pins()
	for _, k := range sortedKeys(cur) {
		cand, ok := c.Images[k]
		switch {
		case !ok:
			pins = append(pins, Pin{Kind: KindImage, Name: k, From: cur[k], Reason: ReasonNoImage})
		case cand.Reason != "":
			pins = append(pins, Pin{Kind: KindImage, Name: k, From: cur[k], Reason: cand.Reason})
		case !sameImage(cur[k], cand.Value):
			pins = append(pins, Pin{Kind: KindImage, Name: k, From: cur[k], To: cand.Value, Resolvable: true, Note: cand.Note})
		}
	}
	slugs := make([]string, 0, len(l.Plugins))
	for s := range l.Plugins {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	for _, s := range slugs {
		from := l.Plugins[s].Version
		cand, ok := c.Plugins[s]
		switch {
		case !ok:
			pins = append(pins, Pin{Kind: KindPlugin, Name: s, From: from, Reason: ReasonNoPlugin})
		case cand.Reason != "":
			pins = append(pins, Pin{Kind: KindPlugin, Name: s, From: from, Reason: cand.Reason})
		case cand.Value != "" && cand.Value != from:
			pins = append(pins, Pin{Kind: KindPlugin, Name: s, From: from, To: cand.Value, Resolvable: true})
		}
	}
	return pins
}

// ApplyImagePins returns a copy of l with every resolvable image pin moved to
// its offered reference. A service that was tag-only becomes digest-pinned.
func ApplyImagePins(l lock.Lock, pins []Pin) lock.Lock {
	out := l
	out.Images = copyMap(l.Images)
	out.Unpinned = copyMap(l.Unpinned)
	for _, p := range pins {
		if p.Kind != KindImage || !p.Resolvable || lock.RefDigest(p.To) == "" {
			continue
		}
		out.Images[p.Name] = p.To
		delete(out.Unpinned, p.Name)
	}
	return out
}

// sameImage reports whether two references name the same image: equal text, or
// the same non-empty digest.
func sameImage(a, b string) bool {
	if a == b {
		return true
	}
	da := lock.RefDigest(a)
	return da != "" && da == lock.RefDigest(b)
}

func trimV(v string) string { return strings.TrimPrefix(strings.TrimSpace(v), "v") }

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
