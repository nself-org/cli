package main

import (
	"strings"
	"testing"
)

// TestReleaseRunRefusedInput is case 2: versions and flags the run refuses, with the
// exact exit code and message.
func TestReleaseRunRefusedInput(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*world)
		args  []string
		code  int
		msg   string
	}{
		{"version equals latest", nil, []string{"--plan", "--version", "1.4.13"}, 2,
			"release: version 1.4.13 is already the latest release; resume a finished release with --from S5 or later\n"},
		{"version older than latest", nil, []string{"--plan", "--version", "1.4.12"}, 2,
			"release: version 1.4.12 is older than the latest release v1.4.13\n"},
		{"existing tag", func(w *world) { w.tags["cli"]["v1.5.0"] = "x" }, []string{"--plan", "--version", "1.5.0"}, 2,
			"release: tag v1.5.0 already exists; resume with --from S4 or later\n"},
		{"existing tag from S3", func(w *world) { w.tags["cli"]["v1.5.0"] = "x" }, []string{"--plan", "--version", "1.5.0", "--from", "S3"}, 2,
			"release: tag v1.5.0 already exists; resume with --from S4 or later\n"},
		{"latest from S4", nil, []string{"--plan", "--version", "1.4.13", "--from", "S4"}, 2,
			"is already the latest release; resume a finished release with --from S5 or later\n"},
		{"older from S5", nil, []string{"--plan", "--version", "1.4.12", "--from", "S5"}, 2,
			"release: version 1.4.12 is older than the latest release v1.4.13\n"},
		{"missing version", nil, []string{"--plan"}, 2, "release: --version X.Y.Z is required (got '')\n"},
		{"bad version", nil, []string{"--plan", "--version", "abc"}, 2, "release: --version X.Y.Z is required (got 'abc')\n"},
		{"bad from", nil, []string{"--plan", "--version", "2.0.0", "--from", "S11"}, 2, "release: --from needs S1..S10\n"},
		{"unknown flag", nil, []string{"--bogus"}, 2, "release: unknown argument: --bogus\nusage: releasectl run"},
		{"latest unreadable", func(w *world) { w.on["latest_fail"] = true }, []string{"--plan", "--version", "2.0.0"}, 1,
			"release: cannot read the latest cli release: gh: HTTP 500\n"},
		{"tag check unreadable", func(w *world) { w.on["tagcheck_fail"] = true }, []string{"--plan", "--version", "2.0.0"}, 1,
			"release: cannot check tag v2.0.0: gh: HTTP 502\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, "", false)
			if c.setup != nil {
				c.setup(f.w)
			}
			wantCode(t, f.run(c.args...), c.code, f)
			if !strings.Contains(f.err.String(), c.msg) {
				t.Errorf("stderr %q lacks %q", f.err.String(), c.msg)
			}
			if f.out.Len() != 0 {
				t.Errorf("stdout not empty: %q", f.out.String())
			}
		})
	}
}

// TestReleaseRunAllowedResumes: a tag present with --from S4, and latest with --from S5.
func TestReleaseRunAllowedResumes(t *testing.T) {
	f := newFixture(t, "", false)
	f.w.tags["cli"]["v1.5.0"] = "x"
	wantCode(t, f.run("--plan", "--version", "1.5.0", "--from", "S4"), 0, f)
	g := newFixture(t, "", false)
	wantCode(t, g.run("--plan", "--version", "1.4.13", "--from", "S5"), 0, g)
	if rows := planRows(g.out.String()); len(rows) == 0 || !strings.HasPrefix(rows[0], "S5 build-wait ") {
		t.Errorf("rows %v", rows)
	}
}

// TestReleaseRunRealRunNeedsTerminal: a real run without a TTY exits 2 and calls nothing.
func TestReleaseRunRealRunNeedsTerminal(t *testing.T) {
	f := newFixture(t, "", false)
	wantCode(t, f.run("--version", "2.0.0"), 2, f)
	stderrHas(t, f, "release: stdin is not a terminal; a real run needs the owner at the keyboard (use --plan to preview)\n")
	if len(f.w.calls) != 0 {
		t.Errorf("calls %v", f.w.calls)
	}
}

// TestRealRunNeedsGH and admin-merge.sh: refused before any call.
func TestReleaseRunRealRunNeedsGHAndAdminMerge(t *testing.T) {
	f := newTTY(t)
	f.d.HaveGH = func() bool { return false }
	wantCode(t, f.run("--version", "2.0.0"), 1, f)
	stderrHas(t, f, "release: gh is not installed\n")
	g := newTTY(t)
	g.d.Root = t.TempDir()
	wantCode(t, g.run("--version", "2.0.0"), 1, g)
	stderrHas(t, g, "release: admin-merge.sh missing next to scripts/release/\n")
	if len(f.w.calls)+len(g.w.calls) != 0 {
		t.Error("calls were made")
	}
}

// TestReleaseRunHelp prints the usage and exits 0.
func TestReleaseRunHelp(t *testing.T) {
	f := newFixture(t, "", false)
	wantCode(t, f.run("--help"), 0, f)
	stderrHas(t, f, "usage: releasectl run --version X.Y.Z")
}

// TestReleaseRunImageWaitBudget is case 18 of cases4: 180 polls x 20 s = 60 minutes.
func TestReleaseRunImageWaitBudget(t *testing.T) {
	if imageWaitPolls*int(pollSeconds.Seconds()) != 3600 {
		t.Errorf("budget %d s", imageWaitPolls*int(pollSeconds.Seconds()))
	}
}
