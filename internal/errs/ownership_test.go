package errs

import (
	"strings"
	"testing"
)

// entryFor builds a valid entry so only ownership can be the problem.
func entryFor(code string) CodeEntry {
	return CodeEntry{Code: code, Summary: "s", DefaultWhy: "w", DefaultFix: "f",
		DocsPath: "reference/error-codes#" + strings.ToLower(code), Exit: 1}
}

// TestOwnership_Mutations proves block ownership is enforced from the Owners
// data: another Epic's number, a spare number and a free number each yield a
// RegistryErrors() entry, while the rightful owner registers cleanly.
func TestOwnership_Mutations(t *testing.T) {
	for _, tc := range []struct {
		name, frag, code string
		wantProblem      bool
	}{
		{"owner registers its own code", "codes_catalog.go", "E115", false},
		{"owner registers last code of its range", "codes_catalog.go", "E117", false},
		{"another Epic claims E116", "codes_plugin_other.go", "E116", true},
		{"owner strays into a neighbour range", "codes_catalog.go", "E121", true},
		{"spare number E062 (TRUTH spare)", "codes_compat.go", "E062", true},
		{"free number E140", "codes_catalog.go", "E140", true},
		{"free number E006 in docker block", "codes_docker.go", "E006", true},
		{"existing fragment grows past its range", "codes_cli.go", "E405", true},
		{"cli codes from the wrong fragment", "codes_docker.go", "E406", true},
		{"spare E440 in cli block", "codes_cli.go", "E440", true},
		{"adopt owner registers shared-proxy code", "codes_shared_proxy.go", "E503", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshotRegistry(t)
			register(tc.frag, entryFor(tc.code))
			got := RegistryErrors()
			if tc.wantProblem {
				if len(got) != 1 || !strings.Contains(got[0].Error(), tc.code) || !strings.Contains(got[0].Error(), tc.frag) {
					t.Fatalf("RegistryErrors() = %v, want one problem naming %s and %s", got, tc.code, tc.frag)
				}
				if _, stored := Registry[tc.code]; stored {
					t.Error("a rejected entry must not be stored")
				}
			} else if len(got) != 0 {
				t.Fatalf("RegistryErrors() = %v, want none", got)
			}
		})
	}
}

// TestOwnership_IntegrityWalkCatchesLeak proves the walk TestRegistryIntegrity
// runs reports a spare number and a wrong-fragment code that reached the
// origin map by some path other than Register, and is clean beforehand.
func TestOwnership_IntegrityWalkCatchesLeak(t *testing.T) {
	snapshotRegistry(t)
	if p := ownershipProblems(); len(p) != 0 {
		t.Fatalf("committed registry has ownership problems: %v", p)
	}
	regMu.Lock()
	regOrigin["E062"] = "codes_compat.go"
	regOrigin["E116"] = "codes_other.go"
	regMu.Unlock()
	if p := ownershipProblems(); len(p) != 2 {
		t.Fatalf("ownershipProblems() = %v, want 2 (spare E062, wrong fragment E116)", p)
	}
}
