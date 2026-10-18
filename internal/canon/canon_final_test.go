package canon

// Final canon ratchet, canon-data side (P7-CANON-17, ADR 0016).
//
// Purpose: once every move has landed the v1.5 view carries no pending command,
// and every old spelling dies at its removal_at release: the lifetime test
// fails the moment the running version reaches a row's removal_at, so the shim
// cannot outlive v1.6.0 by accident.
//
// Constraints: stdlib and internal/version only (L0); the lifetime check takes
// the version as a parameter so the negative subtests inject 1.6.0.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/version"
)

// pendingPaths returns the sorted paths of commands whose canon status is
// still "pending" in f.
func pendingPaths(f *File) []string {
	var out []string
	for path, e := range f.Commands {
		if e.Canon == "pending" {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// lifetimeViolations returns one line per move, shim or retired-hub row whose
// removal_at is at or before current (a release number such as "1.4.14" or
// "v1.6.0"; a pre-release suffix is ignored, so 1.6.0-rc1 counts as 1.6.0). The old spelling must be gone by
// then, so the row must be deleted. Breakouts carry no removal_at: they leave
// the core tree for a plugin and keep no old spelling alive.
func lifetimeViolations(r Rows, current string) ([]string, error) {
	cur, ok := parseVersion("v" + strings.TrimPrefix(strings.SplitN(current, "-", 2)[0], "v"))
	if !ok {
		return nil, fmt.Errorf("version %q is not X.Y.Z", current)
	}
	var out []string
	for _, g := range []struct {
		list string
		rows []Row
	}{{"moves", r.Moves}, {"shims", r.Shims}, {"retired_hubs", r.RetiredHubs}} {
		for _, row := range g.rows {
			end, ok := parseVersion(row.RemovalAt)
			if !ok {
				return nil, fmt.Errorf("%s %q: removal_at %q is not vX.Y.Z", g.list, row.From, row.RemovalAt)
			}
			if !versionLess(cur, end) {
				out = append(out, fmt.Sprintf("%s %q -> %q: removal_at %s reached at %s", g.list, row.From, row.To, row.RemovalAt, current))
			}
		}
	}
	return out, nil
}

func TestCanonNoPending(t *testing.T) {
	t.Run("fixture: a pending entry is named", func(t *testing.T) {
		f := &File{Commands: map[string]Entry{"zeta": {Canon: "pending"}, "alpha": {Canon: "pending"}, "ok": {Canon: "core"}}}
		got := pendingPaths(f)
		if len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
			t.Fatalf("pendingPaths = %v, want [alpha zeta]", got)
		}
	})
	t.Run("v1.5 view", func(t *testing.T) {
		f, err := Effective(true)
		if err != nil {
			t.Fatal(err)
		}
		if got := pendingPaths(f); len(got) != 0 {
			t.Fatalf("%d command(s) still pending in v1.5: %s", len(got), strings.Join(got, ", "))
		}
	})
}

func TestCanonShimLifetime(t *testing.T) {
	rows := Rows{
		Moves:       []Row{{From: "env", To: "config env", Since: "v1.5.0", RemovalAt: "v1.6.0"}},
		Shims:       []Row{{From: "service stop", To: "stop", Since: "v1.5.0", RemovalAt: "v1.6.0", Equivalence: "TestX"}},
		RetiredHubs: []Row{{From: "migrate", To: "db", Since: "v1.5.0", RemovalAt: "v1.6.0"}},
	}
	t.Run("fixture: before removal_at passes", func(t *testing.T) {
		for _, cur := range []string{"1.4.14", "v1.5.0", "1.5.9", "1.5.9-rc1"} {
			if got, err := lifetimeViolations(rows, cur); err != nil || len(got) != 0 {
				t.Fatalf("%s: violations %v, err %v", cur, got, err)
			}
		}
	})
	t.Run("fixture: injected 1.6.0 fails for every row kind", func(t *testing.T) {
		got, err := lifetimeViolations(rows, "1.6.0")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Fatalf("violations = %v, want one per row kind", got)
		}
	})
	t.Run("fixture: a 1.6.0 pre-release already fails", func(t *testing.T) {
		if got, _ := lifetimeViolations(rows, "1.6.0-rc1"); len(got) != 3 {
			t.Fatalf("violations = %v", got)
		}
	})
	t.Run("fixture: a later version fails too", func(t *testing.T) {
		if got, _ := lifetimeViolations(rows, "2.0.0"); len(got) != 3 {
			t.Fatalf("violations = %v", got)
		}
	})
	t.Run("fixture: a bad version is an error", func(t *testing.T) {
		if _, err := lifetimeViolations(rows, "dev"); err == nil {
			t.Fatal("version dev was accepted")
		}
	})
	t.Run("committed canon at the running version", func(t *testing.T) {
		raw, err := LoadRaw()
		if err != nil {
			t.Fatal(err)
		}
		got, err := lifetimeViolations(raw.Rows, version.GetVersion())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("remove these rows (their removal release has arrived):\n%s", strings.Join(got, "\n"))
		}
	})
	t.Run("committed canon at injected 1.6.0 names every row", func(t *testing.T) {
		raw, err := LoadRaw()
		if err != nil {
			t.Fatal(err)
		}
		got, err := lifetimeViolations(raw.Rows, "1.6.0")
		if err != nil {
			t.Fatal(err)
		}
		if want := len(raw.Moves) + len(raw.Shims) + len(raw.RetiredHubs); len(got) != want || want == 0 {
			t.Fatalf("injected 1.6.0 flagged %d of %d rows", len(got), want)
		}
	})
}
