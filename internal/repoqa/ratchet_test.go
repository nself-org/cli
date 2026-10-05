package repoqa

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// update rewrites repoqa ratchet lists, and only ever downward (EPIC G1): it
// lowers counts and drops stale lines. It never adds a line, raises a count or
// rewrites layers.yaml, so a list can only shrink through a visible test edit.
var update = flag.Bool("update", false, "rewrite repoqa ratchet lists downward only")

// countList maps a module-relative path to the number of sites a ratchet
// allowlist tolerates in it.
//
// Purpose:     shared by the layering/orphan guard and the funnel ratchets
//
//	(P7-GUARD-02); one reader, one comparison, one write-down.
type countList map[string]int

// readCountList parses an allowlist file of `<path> <count>` lines. Blank lines
// and `#` comments are ignored. It fails the test on a malformed line, a
// duplicate, or lines that are not in bytewise order (LC_ALL=C sort), so a
// hand edit cannot hide a merge accident.
func readCountList(t testing.TB, path string) countList {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	list := countList{}
	prev := ""
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("%s:%d: %q is not \"<path> <count>\"", path, n, line)
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil || count < 1 {
			t.Fatalf("%s:%d: %q: count must be a positive integer", path, n, line)
		}
		if prev != "" && line <= prev {
			t.Fatalf("%s:%d: %q is out of order or duplicated; keep the file sorted with LC_ALL=C sort", path, n, line)
		}
		prev = line
		list[fields[0]] = count
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return list
}

// compareCountList returns one finding per disagreement between the recorded
// list and the measured counts, sorted. name labels every message (and names
// the funnel the sites should move onto). An empty result means the list is
// tight: every file with sites has a line, and every line equals its count.
func compareCountList(name string, list, actual countList) []string {
	var out []string
	for _, file := range sortedKeys(actual) {
		n := actual[file]
		if n == 0 {
			continue
		}
		m, ok := list[file]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s: %s has %d site(s), not allowlisted; move them onto %s", name, file, n, name))
		case n > m:
			out = append(out, fmt.Sprintf("%s: %s has %d, allowlist says %d; new sites are rejected", name, file, n, m))
		case n < m:
			out = append(out, fmt.Sprintf("%s: %s has %d, allowlist says %d; lower the line to %d", name, file, n, m, n))
		}
	}
	for _, file := range sortedKeys(list) {
		if actual[file] == 0 {
			out = append(out, fmt.Sprintf("%s: %s has 0 sites; delete the line", name, file))
		}
	}
	sort.Strings(out)
	return out
}

// lowerCountLines applies the write-down rule to the lines of an allowlist
// file: a count line is lowered to the measured count (never raised) and is
// dropped when the measured count is 0. Comments and blank lines are kept, no
// line is ever added.
func lowerCountLines(lines []string, actual countList) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		fields := strings.Fields(trim)
		if trim == "" || strings.HasPrefix(trim, "#") || len(fields) != 2 {
			out = append(out, line)
			continue
		}
		recorded, err := strconv.Atoi(fields[1])
		if err != nil {
			out = append(out, line)
			continue
		}
		now := actual[fields[0]]
		switch {
		case now == 0:
			continue
		case now < recorded:
			out = append(out, fmt.Sprintf("%s %d", fields[0], now))
		default:
			out = append(out, line)
		}
	}
	return out
}

// writeCountListDown rewrites path through lowerCountLines, preserving the
// header comment. It writes only when something changed, so -update on a tight
// list leaves the file byte for byte alone. list is the parsed content of path.
func writeCountListDown(t testing.TB, path string, list, actual countList) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	trailing := strings.HasSuffix(string(data), "\n")
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	next := strings.Join(lowerCountLines(lines, actual), "\n")
	if trailing {
		next += "\n"
	}
	if next == string(data) {
		return
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("-update lowered %s (%d line(s) before)", path, len(list))
}

// checkSortedLines returns a finding for each non-comment line that is not
// strictly greater than the one before it (bytewise).
func checkSortedLines(name string, lines []string) []string {
	var out []string
	prev := ""
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if prev != "" && line <= prev {
			out = append(out, fmt.Sprintf("%s: %q is out of order or duplicated; sort with LC_ALL=C sort", name, line))
		}
		prev = line
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestRatchetCompareFindings covers the four findings of a tight count list.
func TestRatchetCompareFindings(t *testing.T) {
	list := countList{"a/above.go": 1, "a/below.go": 3, "a/ok.go": 2, "a/stale.go": 4}
	actual := countList{"a/above.go": 2, "a/below.go": 1, "a/ok.go": 2, "a/new.go": 5}
	got := compareCountList("http-funnel", list, actual)
	want := []string{
		"http-funnel: a/above.go has 2, allowlist says 1; new sites are rejected",
		"http-funnel: a/below.go has 1, allowlist says 3; lower the line to 1",
		"http-funnel: a/new.go has 5 site(s), not allowlisted; move them onto http-funnel",
		"http-funnel: a/stale.go has 0 sites; delete the line",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if extra := compareCountList("x", countList{"a.go": 2}, countList{"a.go": 2}); len(extra) != 0 {
		t.Fatalf("tight list produced findings: %v", extra)
	}
}

// TestRatchetWriteDownNeverAdds proves the write-down only lowers counts and
// drops lines: it never raises a count, never adds a file, keeps comments.
func TestRatchetWriteDownNeverAdds(t *testing.T) {
	path := t.TempDir() + "/list.txt"
	const body = "# header\n\na/above.go 1\na/below.go 3\na/stale.go 4\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	actual := countList{"a/above.go": 9, "a/below.go": 1, "a/new.go": 5}
	writeCountListDown(t, path, readCountList(t, path), actual)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "# header\n\na/above.go 1\na/below.go 1\n"; string(got) != want {
		t.Fatalf("after write-down:\n%q\nwant:\n%q", got, want)
	}
	if strings.Contains(string(got), "new.go") {
		t.Fatal("write-down added a line")
	}
	before := string(got)
	writeCountListDown(t, path, readCountList(t, path), countList{"a/above.go": 1, "a/below.go": 1})
	after, _ := os.ReadFile(path)
	if string(after) != before {
		t.Fatalf("write-down on a tight list changed the file: %q", after)
	}
}

// TestRatchetReadRejectsBadFiles proves a malformed, unsorted or duplicated
// list fails the reader.
func TestRatchetReadRejectsBadFiles(t *testing.T) {
	for name, body := range map[string]string{
		"malformed": "a/x.go\n",
		"zero":      "a/x.go 0\n",
		"unsorted":  "b/x.go 1\na/x.go 1\n",
		"duplicate": "a/x.go 1\na/x.go 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir() + "/list.txt"
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			f := &fakeTB{}
			done := make(chan struct{})
			go func() { defer close(done); readCountList(f, path) }()
			<-done
			if f.fatal == "" {
				t.Fatalf("reader accepted %q", body)
			}
		})
	}
}
