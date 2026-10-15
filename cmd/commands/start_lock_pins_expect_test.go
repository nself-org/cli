package commands

import "testing"

// start_lock_pins_expect_test.go — the fixed expected outcome of every env state
// of the project-lock pin matrix (P7-LIVE-09, round 9).
//
// Purpose: TestProjectLockPinMatrix compares the code with this literal table,
// never with a copy of the rule (a re-implemented rule passed while the rule was
// wrong). Each cell is written by hand: "kept" (the pin is used, no notice) or
// "drop:IMAGE" / "drop:VERSION" (the render moves, one notice names the
// variable of that kind).
// Inputs: the state name and the shape of the row (which variables it reads).
// Outputs: matrixRow.expected.
// Constraints: a *_VERSION equal to the CLI default or to the frozen legacy tag
// (what nself init writes) is not an override; a variable the row does not read
// is inert; a row that reads *_IMAGE (postgres) lets it win over *_VERSION.

// expectBoth is for a row that reads *_IMAGE and *_VERSION (postgres, pgvector).
var expectBoth = map[string]string{
	"d-neither":            "kept",
	"a-image-eq":           "kept",
	"a-image-diff":         "drop:IMAGE",
	"a-image-digest-eq":    "kept",
	"a-image-digest-ne":    "drop:IMAGE",
	"b-version-eq":         "kept",
	"b-version-diff":       "drop:VERSION",
	"b-version-default":    "kept",
	"b-version-legacy":     "kept",
	"c-eq-eq":              "kept",
	"c-eq-diff":            "kept", // the image wins over the version
	"c-diff-eq":            "drop:IMAGE",
	"c-diff-diff":          "drop:IMAGE",
	"e-every-ignored-name": "kept",
}

// expectVersion is for a row that reads only *_VERSION: the *_IMAGE states set a
// variable the generator ignores.
var expectVersion = map[string]string{
	"d-neither":            "kept",
	"a-image-eq":           "kept",
	"a-image-diff":         "kept",
	"a-image-digest-eq":    "kept",
	"a-image-digest-ne":    "kept",
	"b-version-eq":         "kept",
	"b-version-diff":       "drop:VERSION",
	"b-version-default":    "kept",
	"b-version-legacy":     "kept",
	"c-eq-eq":              "kept",
	"c-eq-diff":            "drop:VERSION",
	"c-diff-eq":            "kept",
	"c-diff-diff":          "drop:VERSION",
	"e-every-ignored-name": "kept",
}

// expected returns whether the pin must be kept in state and, when it is
// dropped, the variable the notice names.
func (r *matrixRow) expected(state string) (kept bool, name string) {
	if _, known := expectVersion[state]; !known {
		panic("unknown project-lock pin matrix state: " + state)
	}
	table := map[string]string{} // a row that reads no variable keeps its pin in every state
	switch {
	case r.feeds(r.iVar) && r.feeds(r.vVar):
		table = expectBoth
	case r.feeds(r.vVar):
		table = expectVersion
	}
	want := "kept"
	if len(table) != 0 {
		want = table[state]
	}
	switch want {
	case "kept":
		return true, ""
	case "drop:IMAGE":
		return false, r.iVar
	case "drop:VERSION":
		return false, r.vVar
	default:
		panic("unknown project-lock pin matrix expectation: " + want)
	}
}

func TestProjectLockPinMatrixRejectsUnknownExpectationState(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("an unknown matrix state must not silently default to kept")
		}
	}()
	(&matrixRow{}).expected("misspelled-state")
}
