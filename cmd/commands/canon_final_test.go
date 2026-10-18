package commands

// Final canon ratchet, command-tree side (P7-CANON-17, ADR 0016).
//
// Purpose: pin the two facts of the finished v1.5 canon that only the cobra
// tree can prove: the root help fits one screen and the registry counts the 20
// core verbs with none missing.
//
// Constraints: both tests select v1.5 themselves, so they pass whatever mode
// the process starts in; the negative subtests run the same checks on fixtures.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat"
)

// helpScreenLines is the one-screen budget for `nself --help` (ADR 0016).
const helpScreenLines = 45

// checkHelpFits reports an error when text has more than max lines.
func checkHelpFits(text string, max int) error {
	if n := strings.Count(text, "\n"); n > max {
		return fmt.Errorf("root help is %d lines, the one-screen budget is %d", n, max)
	}
	return nil
}

// checkCoreCount reports an error unless the counts show want core verbs, none
// missing, none pending and at most want visible top-level commands.
func checkCoreCount(c cmdregistry.Counts, want int) error {
	switch {
	case c.Core != want:
		return fmt.Errorf("counts.core = %d, want %d", c.Core, want)
	case len(c.CoreMissing) != 0:
		return fmt.Errorf("core verbs missing from the tree: %v", c.CoreMissing)
	case c.Pending != 0:
		return fmt.Errorf("counts.pending = %d, want 0", c.Pending)
	case c.TopLevel > want:
		return fmt.Errorf("counts.top_level = %d exceeds the %d core verbs", c.TopLevel, want)
	}
	return nil
}

func TestHelpFits(t *testing.T) {
	t.Run("fixture: 46 lines fails", func(t *testing.T) {
		if err := checkHelpFits(strings.Repeat("x\n", helpScreenLines+1), helpScreenLines); err == nil {
			t.Fatal("a 46-line help passed")
		}
	})
	t.Run("fixture: 45 lines passes", func(t *testing.T) {
		if err := checkHelpFits(strings.Repeat("x\n", helpScreenLines), helpScreenLines); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("v1.5 root help", func(t *testing.T) {
		t.Setenv(compat.EnvVar, "1")
		reattachRealTree()
		defer PrepareTreeForGeneration()()
		var out bytes.Buffer
		RootCmd.SetOut(&out)
		defer RootCmd.SetOut(nil)
		if err := RootCmd.Help(); err != nil {
			t.Fatal(err)
		}
		if err := checkHelpFits(out.String(), helpScreenLines); err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
	})
}

func TestCanonCoreCount(t *testing.T) {
	t.Run("fixture: 19 core verbs fails", func(t *testing.T) {
		if err := checkCoreCount(cmdregistry.Counts{Core: 19, TopLevel: 19}, 20); err == nil {
			t.Fatal("19 core verbs passed")
		}
	})
	t.Run("fixture: a missing verb fails", func(t *testing.T) {
		c := cmdregistry.Counts{Core: 20, TopLevel: 20, CoreMissing: []string{"exec"}}
		if err := checkCoreCount(c, 20); err == nil {
			t.Fatal("a missing core verb passed")
		}
	})
	t.Run("fixture: a pending command fails", func(t *testing.T) {
		if err := checkCoreCount(cmdregistry.Counts{Core: 20, TopLevel: 20, Pending: 1}, 20); err == nil {
			t.Fatal("a pending command passed")
		}
	})
	t.Run("fixture: 21 top-level commands fails", func(t *testing.T) {
		if err := checkCoreCount(cmdregistry.Counts{Core: 20, TopLevel: 21}, 20); err == nil {
			t.Fatal("21 top-level commands passed")
		}
	})
	t.Run("v1.5 registry", func(t *testing.T) {
		t.Setenv(compat.EnvVar, "1")
		reg, err := BuildRegistry(true)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkCoreCount(reg.Counts, 20); err != nil {
			t.Fatal(err)
		}
	})
}
