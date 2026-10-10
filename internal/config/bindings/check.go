package bindings

// check.go — the bindings gate.
//
// Purpose: fail when a flag named like a known configuration key is neither
//          bound nor exempt, and when a binding or exemption names a flag
//          that does not exist.
// Inputs:  every flag of the command tree (FlagRef), the known key list.
// Outputs: one error listing every violation, or nil.
// Constraints: pure; the caller supplies the flags (the registry, or a
//              fixture tree in tests).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// FlagRef names one declared flag of one command (canonical path, or Root).
type FlagRef struct {
	Command string
	Flag    string
}

// CandidateKey reports the known key a flag name looks like: the upper-snake
// name, with or without the NSELF_ prefix.
func CandidateKey(flag string, known []string) (string, bool) {
	upper := strings.ToUpper(strings.ReplaceAll(flag, "-", "_"))
	for _, want := range []string{upper, "NSELF_" + upper} {
		for _, k := range known {
			if k == want {
				return k, true
			}
		}
	}
	return "", false
}

// Check validates the bindings against the flags that exist. It reports an
// unbound candidate, a binding or exemption for a missing flag, and nothing
// else; key validity is Parse's job.
func (b Bindings) Check(flags []FlagRef) error {
	known := config.KnownEnvVars()
	exists := map[string]bool{}
	for _, f := range flags {
		exists[ref(f.Command, f.Flag)] = true
	}
	var problems []string
	for _, f := range flags {
		key, ok := CandidateKey(f.Flag, known)
		if !ok {
			continue
		}
		if _, bound := b.Lookup(f.Command, f.Flag); bound {
			continue
		}
		if _, exempt := b.Exemption(f.Command, f.Flag); exempt {
			continue
		}
		problems = append(problems, fmt.Sprintf("nself %s --%s looks like configuration key %s: bind it in internal/config/bindings/flags.yaml or exempt it with a reason in exempt.yaml", f.Command, f.Flag, key))
	}
	for _, r := range b.Flags {
		if !exists[ref(r.Command, r.Flag)] {
			problems = append(problems, fmt.Sprintf("binding %s --%s -> %s: no such flag on that command", r.Command, r.Flag, r.Key))
		}
	}
	for _, r := range b.Exempt {
		if !exists[ref(r.Command, r.Flag)] {
			problems = append(problems, fmt.Sprintf("exemption %s --%s: no such flag on that command", r.Command, r.Flag))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("config bindings gate:\n  %s", strings.Join(problems, "\n  "))
}
