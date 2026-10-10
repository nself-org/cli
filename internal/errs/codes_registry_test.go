package errs

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// snapshotRegistry saves the package registry state and restores it when the
// test ends, so test-local Register calls never leak into TestRegistryIntegrity.
func snapshotRegistry(t *testing.T) {
	t.Helper()
	regMu.Lock()
	reg := make(map[string]CodeEntry, len(Registry))
	for k, v := range Registry {
		reg[k] = v
	}
	org := make(map[string]string, len(regOrigin))
	for k, v := range regOrigin {
		org[k] = v
	}
	probs := append([]error(nil), regProblems...)
	regMu.Unlock()
	t.Cleanup(func() {
		regMu.Lock()
		defer regMu.Unlock()
		Registry, regOrigin, regProblems = reg, org, probs
	})
}

// forgetCode removes code from Registry and the origin table inside a
// snapshotRegistry test, so a test may use a number a real fragment has since
// registered (E500-E502 once P7-ADOPT-01 landed) without a duplicate. The
// snapshot cleanup restores the real entry.
func forgetCode(code string) {
	regMu.Lock()
	defer regMu.Unlock()
	delete(Registry, code)
	delete(regOrigin, code)
}

// TestRegistryIntegrity fails when any fragment registered a duplicate code, a
// code outside its block, a mismatched category, a reserved-block code or an
// invalid exit class. Later Tickets rely on it to catch collisions.
func TestRegistryIntegrity(t *testing.T) {
	for _, err := range RegistryErrors() {
		t.Errorf("registry problem: %v", err)
	}
	if len(Registry) == 0 {
		t.Fatal("Registry is empty: no fragment registered")
	}
	for _, p := range ownershipProblems() {
		t.Error(p)
	}
}

// ownershipProblems is a belt-and-braces walk over Register's own check:
// every committed code lies in an owner range and came from that owner's
// fragment.
func ownershipProblems() []string {
	regMu.Lock()
	defer regMu.Unlock()
	var out []string
	for code, frag := range regOrigin {
		n, _ := strconv.Atoi(code[1:])
		o, ok := ownerFor(n)
		if !ok {
			out = append(out, fmt.Sprintf("%s (from %s) is in no owner range: spare or free numbers need an Epic table line first", code, frag))
		} else if o.Fragment != frag {
			out = append(out, fmt.Sprintf("%s registered from %s, but %s (%s) owns it", code, frag, o.Fragment, o.Who))
		}
	}
	return out
}

var docsAnchorRe = regexp.MustCompile(`^reference/error-codes#e[0-9]{3}$`)

// TestRegistryEntries_Contract checks every Registry entry: code format and
// key match, category range, exit class, docs anchor and non-empty guidance.
func TestRegistryEntries_Contract(t *testing.T) {
	for code, e := range Registry {
		if !codeRe.MatchString(code) || e.Code != code {
			t.Errorf("%s: key/Code mismatch (entry Code %q)", code, e.Code)
		}
		n, _ := strconv.Atoi(code[1:])
		blk, ok := blockFor(n)
		if !ok || blk.Category != e.Category {
			t.Errorf("%s: category %q not the block category (%q, in block %v)", code, e.Category, blk.Category, ok)
		}
		if e.Exit < ExitUserError || e.Exit > ExitDestructiveBlocked {
			if !(code == "E400" && e.Exit == 0) {
				t.Errorf("%s: invalid Exit %d", code, e.Exit)
			}
		}
		if want := "reference/error-codes#" + strings.ToLower(code); e.DocsPath != want || !docsAnchorRe.MatchString(e.DocsPath) {
			t.Errorf("%s: DocsPath %q, want %q", code, e.DocsPath, want)
		}
		if e.Summary == "" || e.DefaultWhy == "" || e.DefaultFix == "" {
			t.Errorf("%s: Summary/DefaultWhy/DefaultFix must be non-empty", code)
		}
	}
}

// TestRegistryExitTable pins the exit class of every range to the normative
// Epic table, including the exceptions.
func TestRegistryExitTable(t *testing.T) {
	want := func(code string) int {
		n, _ := strconv.Atoi(code[1:])
		switch {
		case n < 50, n >= 150 && n < 200, n >= 250 && n < 300:
			return 2
		case n >= 200 && n < 250:
			if n == 203 {
				return 1
			}
			return 2
		case n >= 100 && n < 150:
			if (n >= 101 && n <= 104) || n == 110 {
				return 3
			}
			if n == 118 || n == 119 { // P7-PLUG-59: readiness failures are infra (plugin did not become ready)
				return 2
			}
			return 1
		case n == 400:
			return 0
		case n == 422 || n == 423: // P7-SURF-24: no single JSON document and invocation timeout are infra (EPIC SURF D14)
			return 2
		case n == 403:
			return 4
		}
		return 1
	}
	for code, e := range Registry {
		if got := want(code); e.Exit != got {
			t.Errorf("%s: Exit %d, want %d", code, e.Exit, got)
		}
	}
}

// TestRegistry_E404 pins the relocation-refusal code.
func TestRegistry_E404(t *testing.T) {
	e, ok := Registry["E404"]
	if !ok {
		t.Fatal("E404 is not registered")
	}
	if e.Exit != 1 || e.DocsPath != "reference/error-codes#e404" || e.Category != "cli" {
		t.Errorf("E404 = %+v, want Exit 1, cli, anchor e404", e)
	}
}

// TestRegistry_OriginGolden proves the fragment split changed no meaning:
// Code/Category/Summary/DefaultWhy/DefaultFix of the origin 29 entries match.
func TestRegistry_OriginGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/registry_origin.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct{ Code, Category, Summary, DefaultWhy, DefaultFix string }
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 29 {
		t.Fatalf("golden has %d rows, want 29", len(rows))
	}
	for _, r := range rows {
		got, ok := Registry[r.Code]
		if !ok {
			t.Errorf("%s: missing from Registry", r.Code)
			continue
		}
		if got.Category != r.Category || got.Summary != r.Summary || got.DefaultWhy != r.DefaultWhy || got.DefaultFix != r.DefaultFix {
			t.Errorf("%s: changed meaning: got %+v", r.Code, got)
		}
	}
}

// TestRegister_DuplicateNamesBothFragments registers an existing code again
// and expects a problem naming the code and both fragment files, no panic,
// and the first registration kept.
func TestRegister_DuplicateNamesBothFragments(t *testing.T) {
	snapshotRegistry(t)
	before := Registry["E002"]
	Register(CodeEntry{Code: "E002", Summary: "x", DefaultWhy: "x", DefaultFix: "x", DocsPath: "reference/error-codes#e002", Exit: 2})
	problems := RegistryErrors()
	if len(problems) != 1 {
		t.Fatalf("RegistryErrors() = %v, want one entry", problems)
	}
	msg := problems[0].Error()
	for _, part := range []string{"E002", "codes_docker.go", "codes_registry_test.go"} {
		if !strings.Contains(msg, part) {
			t.Errorf("problem %q does not name %q", msg, part)
		}
	}
	if Registry["E002"] != before {
		t.Error("first registration must win")
	}
}

// TestRegister_RejectsBadCodes covers reserved, unallocated and mismatched
// registrations plus the empty-category fill.
func TestRegister_RejectsBadCodes(t *testing.T) {
	ok := func(code, cat string) CodeEntry {
		return CodeEntry{Code: code, Category: cat, Summary: "s", DefaultWhy: "w", DefaultFix: "f", DocsPath: "reference/error-codes#" + strings.ToLower(code), Exit: 1}
	}
	for _, tc := range []struct {
		name string
		e    CodeEntry
	}{
		{"reserved external", ok("E650", "")},
		{"no block", ok("E535", "")},
		{"category differs", ok("E500", "config")},
		{"bad format", ok("X12", "")},
		{"bad exit", CodeEntry{Code: "E501", Summary: "s", Exit: 9}},
		{"exit 0 off E400", CodeEntry{Code: "E502", Summary: "s", Exit: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshotRegistry(t)
			forgetCode(tc.e.Code)
			Register(tc.e)
			if n := len(RegistryErrors()); n != 1 {
				t.Fatalf("RegistryErrors() has %d entries, want 1", n)
			}
			if _, stored := Registry[tc.e.Code]; stored {
				t.Error("a rejected entry must not be stored")
			}
		})
	}
	t.Run("empty category filled from block", func(t *testing.T) {
		snapshotRegistry(t)
		forgetCode("E500")
		Register(ok("E500", ""))
		if n := len(RegistryErrors()); n != 0 {
			t.Fatalf("RegistryErrors() = %v, want none", RegistryErrors())
		}
		if got := Registry["E500"].Category; got != "adopt" {
			t.Errorf("E500 category = %q, want adopt", got)
		}
	})
}

// TestBlockTables proves blocks are disjoint and clear of the reserved block,
// and that owner ranges are disjoint and each nested inside one block.
func TestBlockTables(t *testing.T) {
	bs := append([]Block(nil), Blocks...)
	sort.Slice(bs, func(i, j int) bool { return bs[i].Lo < bs[j].Lo })
	for i, b := range bs {
		if b.Lo > b.Hi || b.Category == "" {
			t.Errorf("bad block %+v", b)
		}
		if i > 0 && b.Lo <= bs[i-1].Hi {
			t.Errorf("blocks overlap: %+v and %+v", bs[i-1], b)
		}
		if b.Lo <= reservedHi && b.Hi >= reservedLo {
			t.Errorf("block %+v reaches the reserved external block", b)
		}
	}
	owns := append([]Owner(nil), Owners...)
	sort.Slice(owns, func(i, j int) bool { return owns[i].Lo < owns[j].Lo })
	for i, o := range owns {
		if o.Lo > o.Hi || o.Who == "" || !strings.HasPrefix(o.Fragment, "codes_") || !strings.HasSuffix(o.Fragment, ".go") {
			t.Errorf("bad owner range %+v", o)
		}
		if i > 0 && o.Lo <= owns[i-1].Hi {
			t.Errorf("owner ranges overlap: %+v and %+v", owns[i-1], o)
		}
		b1, ok1 := blockFor(o.Lo)
		b2, ok2 := blockFor(o.Hi)
		if !ok1 || !ok2 || b1 != b2 {
			t.Errorf("owner range %+v is not nested inside one block", o)
		}
	}
}
