package reconcile

import "sort"

// MaxDiffLines is the per-side line cap. When either side of a changed
// artifact is longer, Diff reports the change with DiffLines = -1 and Unified
// returns no text, which bounds the cost of a plan over huge files.
const MaxDiffLines = 20000

// Diff compares two artifact sets and returns the changed items.
//
// Inputs: before (what is on disk) and after (what the pipeline rendered);
// handEdited reports whether a path was changed by a human since nself last
// wrote it (nil means never). Outputs: add, change and remove items sorted by
// path; unchanged files are omitted. Only file bytes are compared: a
// permission-only difference is not a change here. generated is true when
// either side carries the nself marker; hand_edited is only set on change and
// remove (an add has nothing to overwrite); redacted is true for env-kind
// paths. DiffLines is inserted plus deleted lines (a change of one line is 2),
// the whole line count for add and remove, and -1 (for any action) over
// MaxDiffLines on a side. A change whose edit distance passes maxEditDistance
// is also -1: the count is not computed past that bound. The action of an add
// or remove over the cap stays add/remove: it describes what happens to the
// file, and -1 only says the size is not reported.
func Diff(before, after ArtifactSet, handEdited func(path string) bool) []Artifact {
	paths := make([]string, 0, len(before)+len(after))
	for p := range before {
		paths = append(paths, p)
	}
	for p := range after {
		if _, dup := before[p]; !dup {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	out := make([]Artifact, 0)
	for _, p := range paths {
		bf, inBefore := before[p]
		af, inAfter := after[p]
		item := Artifact{Kind: Kind(p), Path: p, Redacted: IsEnvPath(p)}
		switch {
		case !inBefore:
			item.Action = ActionAdd
			item.DiffLines = sideLines(af.Data)
		case !inAfter:
			item.Action = ActionRemove
			item.DiffLines = sideLines(bf.Data)
		case string(bf.Data) == string(af.Data):
			continue
		default:
			item.Action = ActionChange
			item.DiffLines = changedLines(bf.Data, af.Data)
		}
		item.Generated = (inBefore && IsGenerated(bf.Data)) || (inAfter && IsGenerated(af.Data))
		item.HandEdited = inBefore && handEdited != nil && handEdited(p)
		out = append(out, item)
	}
	return out
}

// sideLines returns the line count of a wholly added or removed file, or -1
// over MaxDiffLines.
func sideLines(data []byte) int {
	if n := countLines(data); n <= MaxDiffLines {
		return n
	}
	return -1
}

// changedLines returns the inserted plus deleted line count between a and b,
// or -1 when either side exceeds MaxDiffLines or the edit distance passes
// maxEditDistance.
func changedLines(a, b []byte) int {
	if countLines(a) > MaxDiffLines || countLines(b) > MaxDiffLines {
		return -1
	}
	x, y := intern(splitLines(a), splitLines(b))
	d, _ := myers(x, y, false)
	return d
}
