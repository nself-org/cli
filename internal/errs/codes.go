package errs

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"sync"
)

// CodeEntry defines a registered error code: its identity, its default
// guidance fields, and the process exit class the code carries.
//
// Purpose: one row of contract:cli.error-codes v1 and contract:cli.exit-codes
// v1. Fragment files (codes_<topic>.go) build CodeEntry values and pass them
// to Register from init().
//
// Inputs: Code must match ^E[0-9]{3}$ and lie inside one block of the table in
// codes_blocks.go. Exit is one of ExitUserError..ExitDestructiveBlocked; 0 is
// allowed only on E400, where it means "classify by sentinel, else 1".
//
// Outputs: the stored value is what Registry[code] returns. Register fills an
// empty Category from the code's block and always sets Code.
//
// Constraints: a code is never renumbered or reused with a different meaning.
type CodeEntry struct {
	// Code is the error code, e.g. "E002". Set by the fragment; Register
	// uses it as the Registry key.
	Code string

	// Category groups related errors (e.g., "docker", "config", "plugin").
	// Optional in a fragment: it is derived from the code's block.
	Category string

	// Summary is a short description of this error class.
	Summary string

	// DefaultWhy is the default root-cause explanation.
	DefaultWhy string

	// DefaultFix is the default remediation steps.
	DefaultFix string

	// DocsPath is the docs URL path fragment.
	DocsPath string

	// Exit is the process exit status for this code (1 user, 2 infra, 3 auth,
	// 4 destructive_blocked). 0 is valid only for E400.
	Exit int
}

// Registry maps error codes (E001..E999) to their metadata. It is filled by
// Register calls from the codes_<topic>.go fragment files during package
// init; treat it as read-only afterwards. New codes are added in a new
// fragment file inside the owning Epic's range (see codes_blocks.go), never by
// editing this map.
var Registry = map[string]CodeEntry{}

var (
	codeRe = regexp.MustCompile(`^E[0-9]{3}$`)

	regMu       sync.Mutex
	regOrigin   = map[string]string{} // code -> fragment file that registered it
	regProblems []error
)

// Register adds entries to Registry. It is called from init() in a fragment
// file and never panics (Constitution §4.2).
//
// Purpose: single entry point for contributing codes, so parallel Tickets
// each own one fragment file and never edit a shared table.
//
// Inputs: entries with a Code, Summary-level fields, DocsPath and Exit.
//
// Outputs: valid entries are stored (first registration wins). Every problem
// (bad code format, code outside every block, code in the reserved external
// block, category that differs from the block's, duplicate code, invalid Exit)
// is appended to the problem list instead, naming the code and the fragment
// file(s). RegistryErrors returns that list and TestRegistryIntegrity fails
// on it.
//
// Constraints: the fragment name is the base name of the calling source file.
func Register(entries ...CodeEntry) {
	frag := "unknown"
	if _, file, _, ok := runtime.Caller(1); ok {
		frag = filepath.Base(file)
	}
	regMu.Lock()
	defer regMu.Unlock()
	for _, e := range entries {
		if err := checkEntry(&e); err != nil {
			regProblems = append(regProblems, fmt.Errorf("%s: %s: %w", frag, e.Code, err))
			continue
		}
		if first, dup := regOrigin[e.Code]; dup {
			regProblems = append(regProblems, fmt.Errorf(
				"%s: duplicate error code %s (already registered by %s)", frag, e.Code, first))
			continue
		}
		Registry[e.Code] = e
		regOrigin[e.Code] = frag
	}
}

// checkEntry validates one entry and fills an empty Category from its block.
func checkEntry(e *CodeEntry) error {
	if !codeRe.MatchString(e.Code) {
		return fmt.Errorf("code must match ^E[0-9]{3}$")
	}
	n, _ := strconv.Atoi(e.Code[1:])
	if n >= reservedLo && n <= reservedHi {
		return fmt.Errorf("code is inside the reserved external block E%d-E%d", reservedLo, reservedHi)
	}
	blk, ok := blockFor(n)
	if !ok {
		return fmt.Errorf("code is not inside any allocated block")
	}
	if e.Category == "" {
		e.Category = blk.Category
	} else if e.Category != blk.Category {
		return fmt.Errorf("category %q differs from the block category %q", e.Category, blk.Category)
	}
	if e.Exit < ExitUserError || e.Exit > ExitDestructiveBlocked {
		if e.Exit != 0 || e.Code != "E400" {
			return fmt.Errorf("exit %d is not a valid class (1-4; 0 only for E400)", e.Exit)
		}
	}
	return nil
}

// RegistryErrors returns every registration problem recorded so far: a
// duplicate code, a code outside its block, a mismatched category, and so on.
// It is empty when the committed fragments are consistent.
func RegistryErrors() []error {
	regMu.Lock()
	defer regMu.Unlock()
	out := make([]error, len(regProblems))
	copy(out, regProblems)
	return out
}

// Categories returns all unique category names from the registry. The result
// is sorted so callers see a stable order.
func Categories() []string {
	seen := make(map[string]bool)
	var cats []string
	for _, entry := range Registry {
		if !seen[entry.Category] {
			seen[entry.Category] = true
			cats = append(cats, entry.Category)
		}
	}
	sort.Strings(cats)
	return cats
}
