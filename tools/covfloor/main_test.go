package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// cov runs covfloor in-process and returns exit code, stdout and stderr.
func cov(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, fixedNow)
	return code, out.String(), errb.String()
}

// tmpFile writes content into a temp dir and returns the path.
func tmpFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPassWhenEveryPackageIsAtOrAboveItsFloor(t *testing.T) {
	code, out, errs := cov(t, "-module", "example.com/mod", "-profile", "testdata/pass.out", "-floors", "testdata/pass-floors.txt")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	for _, want := range []string{"PASS . 100.0% floor 100%", "PASS internal/a 80.0% floor 78%", "PASS internal/b 50.0% floor 50%"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestBelowFloorFails(t *testing.T) {
	code, out, _ := cov(t, "-module", "github.com/nself-org/cli", "-profile", "testdata/below.out", "-floors", "testdata/floors.txt")
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, out)
	}
	if !regexp.MustCompile(`(?m)^FAIL internal/license 50\.0% floor 60%$`).MatchString(out) ||
		!strings.Contains(out, "PASS internal/auth 90.0% floor 80%") {
		t.Errorf("unexpected report:\n%s", out)
	}
}

func TestModuleIsReadFromGoMod(t *testing.T) {
	gomod := tmpFile(t, "go.mod", "module github.com/nself-org/cli\n\ngo 1.26\n")
	code, _, _ := cov(t, "-gomod", gomod, "-profile", "testdata/below.out", "-floors", "testdata/floors.txt")
	if code != 1 {
		t.Errorf("exit %d, want 1 (license below floor)", code)
	}
}

func TestStaleFloorLineFails(t *testing.T) {
	floors := tmpFile(t, "f.txt", "internal/a 70\ninternal/b 40\n.  90\ninternal/gone 10\n")
	code, out, _ := cov(t, "-module", "example.com/mod", "-profile", "testdata/pass.out", "-floors", floors)
	if code != 1 || !strings.Contains(out, "STALE internal/gone floor 10%") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestUnflooredIsPrintedAndOnlyFailsInStrictMode(t *testing.T) {
	floors := tmpFile(t, "f.txt", "internal/a 70\n")
	args := []string{"-module", "example.com/mod", "-profile", "testdata/pass.out", "-floors", floors}
	code, out, _ := cov(t, args...)
	if code != 0 || !strings.Contains(out, "UNFLOORED internal/b 50.0% floor none") || !strings.Contains(out, "UNFLOORED . 100.0%") {
		t.Errorf("default: exit %d\n%s", code, out)
	}
	if code, _, _ := cov(t, append(args, "-strict")...); code != 1 {
		t.Errorf("strict: exit %d, want 1", code)
	}
}

func TestWriteAppendsMissingAndNeverChangesExistingLines(t *testing.T) {
	orig := "# header\n\ninternal/b 40  # keep me exactly\n# lead comment of a\ninternal/a   99\n"
	floors := tmpFile(t, "f.txt", orig)
	args := []string{"-module", "example.com/mod", "-profile", "testdata/pass.out", "-floors", floors, "-write"}
	// internal/a is below its 99 floor: -write must report the failure, not fix it.
	code, _, errs := cov(t, args...)
	if code != 1 || !strings.Contains(errs, "wrote 1 new floor line(s)") {
		t.Fatalf("exit %d: %s", code, errs)
	}
	got, _ := os.ReadFile(floors)
	text := string(got)
	for _, line := range []string{"internal/b 40  # keep me exactly", "# lead comment of a", "internal/a   99", ". 98  # measured 100.0 2026-10-05"} {
		if !strings.Contains(text, line+"\n") {
			t.Errorf("missing line %q in\n%s", line, text)
		}
	}
	if !strings.HasPrefix(text, "# header\n\n") {
		t.Errorf("header lost:\n%s", text)
	}
	// Sorted bytewise, and the lead comment travels with internal/a.
	if i, j, k := strings.Index(text, ". 98"), strings.Index(text, "# lead comment of a\ninternal/a"), strings.Index(text, "internal/b"); !(i < j && j < k) {
		t.Errorf("not sorted with lead comments attached:\n%s", text)
	}
	// A second -write has nothing to add and leaves the file byte-identical.
	cov(t, args...)
	again, _ := os.ReadFile(floors)
	if string(again) != text {
		t.Errorf("second -write changed the file:\n%s", again)
	}
}

func TestNewFloor(t *testing.T) {
	for _, c := range []struct {
		pct  float64
		want int
	}{{61.9, 59}, {100, 98}, {2.0, 0}, {1.5, 0}, {0, 0}, {37.4, 35}} {
		if got := newFloor(c.pct); got != c.want {
			t.Errorf("newFloor(%v) = %d, want %d", c.pct, got, c.want)
		}
	}
}

func TestFailClosed(t *testing.T) {
	okFloors := tmpFile(t, "floors.txt", "internal/a 10\n")
	cases := []struct {
		name, profile, floors string
		want                  string
	}{
		{"missing profile", "testdata/does-not-exist.out", okFloors, "profile:"},
		{"empty profile", tmpFile(t, "e.out", ""), okFloors, "empty"},
		{"header only", tmpFile(t, "h.out", "mode: atomic\n"), okFloors, "no statements"},
		{"bad mode", tmpFile(t, "m.out", "nonsense\nexample.com/mod/internal/a/a.go:1.1,2.2 1 1\n"), okFloors, "mode"},
		{"malformed line", tmpFile(t, "l.out", "mode: set\nthis is not a block\n"), okFloors, "line 2"},
		{"non-numeric count", tmpFile(t, "n.out", "mode: set\nexample.com/mod/internal/a/a.go:1.1,2.2 1 x\n"), okFloors, "hit count"},
		{"file outside module", tmpFile(t, "o.out", "mode: set\nother.org/x/x.go:1.1,2.2 1 1\n"), okFloors, "outside module"},
		{"inconsistent block", tmpFile(t, "i.out", "mode: set\nexample.com/mod/internal/a/a.go:1.1,2.2 1 1\nexample.com/mod/internal/a/a.go:1.1,2.2 2 1\n"), okFloors, "repeated"},
		{"missing floors", "testdata/pass.out", "testdata/nope.txt", "floors:"},
		{"empty floors", "testdata/pass.out", tmpFile(t, "ef.txt", "# nothing\n"), "no package lines"},
		{"bad floor value", "testdata/pass.out", tmpFile(t, "bf.txt", "internal/a lots\n"), "line 1"},
		{"floor above 100", "testdata/pass.out", tmpFile(t, "hf.txt", "internal/a 101\n"), "0 to 100"},
		{"duplicate floor", "testdata/pass.out", tmpFile(t, "df.txt", "internal/a 1\ninternal/a 2\n"), "already has a floor"},
		{"extra column", "testdata/pass.out", tmpFile(t, "xf.txt", "internal/a 1 2\n"), "want `<package> <floor>`"},
	}
	for _, c := range cases {
		code, out, errs := cov(t, "-module", "example.com/mod", "-profile", c.profile, "-floors", c.floors)
		if code != 1 || !strings.Contains(errs, c.want) {
			t.Errorf("%s: exit %d, stderr %q, want exit 1 containing %q (stdout %q)", c.name, code, errs, c.want, out)
		}
	}
	if code, _, errs := cov(t, "-gomod", "testdata/no-go.mod", "-profile", "testdata/pass.out", "-floors", okFloors); code != 1 || !strings.Contains(errs, "module path") {
		t.Errorf("missing go.mod: exit %d %q", code, errs)
	}
}

func TestUsageErrorsExit2(t *testing.T) {
	if code, _, _ := cov(t); code != 2 {
		t.Errorf("no flags: exit %d", code)
	}
	if code, _, _ := cov(t, "-bogus"); code != 2 {
		t.Errorf("bad flag: exit %d", code)
	}
}

func TestJSONShape(t *testing.T) {
	floors := tmpFile(t, "f.txt", "internal/a 70\n")
	code, out, _ := cov(t, "-json", "-module", "example.com/mod", "-profile", "testdata/pass.out", "-floors", floors)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var d struct {
		Packages []struct {
			Package  string  `json:"package"`
			Coverage float64 `json:"coverage"`
			Floor    *int    `json:"floor"`
			Status   string  `json:"status"`
		} `json:"packages"`
		Failed, Stale, Unfloored []string
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Packages) != 3 || d.Packages[1].Package != "internal/a" || d.Packages[1].Coverage != 80 ||
		d.Packages[1].Floor == nil || *d.Packages[1].Floor != 70 || d.Packages[1].Status != "PASS" ||
		d.Packages[0].Floor != nil || d.Packages[0].Status != "UNFLOORED" {
		t.Errorf("packages: %+v", d.Packages)
	}
	if d.Failed == nil || d.Stale == nil || len(d.Unfloored) != 2 {
		t.Errorf("failed %v stale %v unfloored %v (slices must be [] not null)", d.Failed, d.Stale, d.Unfloored)
	}
	if !strings.Contains(out, `"failed": []`) || !strings.Contains(out, `"stale": []`) {
		t.Errorf("empty slices must render as []:\n%s", out)
	}
}

func TestRepeatedBlockCountsOnceAndAnyHitCovers(t *testing.T) {
	p := tmpFile(t, "r.out", "mode: count\n"+
		"example.com/mod/internal/a/a.go:1.1,2.2 4 0\n"+
		"example.com/mod/internal/a/a.go:1.1,2.2 4 7\n"+
		"example.com/mod/internal/a/a.go:3.1,4.2 6 0\n")
	m, err := ReadProfile(p, "example.com/mod")
	if err != nil {
		t.Fatal(err)
	}
	if c := m["internal/a"]; c.Total != 10 || c.Covered != 4 {
		t.Errorf("got %+v, want 4/10", c)
	}
}

// TestMatchesGoToolCoverFunc builds a throwaway module, profiles it with the
// real toolchain and checks covfloor's per-package percentage against the
// `total:` line of `go tool cover -func` (one package per profile).
func TestMatchesGoToolCoverFunc(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	dir := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/fix\n\ngo 1.22\n")
	write("p/p.go", `package p

func Pick(n int) string {
	if n > 0 {
		return "pos"
	} else if n < 0 {
		return "neg"
	}
	for i := 0; i < 3; i++ {
		n += i
	}
	return "zero"
}

func Unused() int {
	x := 1
	x++
	return x
}
`)
	write("p/p_test.go", "package p\n\nimport \"testing\"\n\nfunc TestPick(t *testing.T) { _ = Pick(1); _ = Pick(0) }\n")
	env := append(os.Environ(), "GOFLAGS=", "GOWORK=off", "CGO_ENABLED=0")
	sh := func(args ...string) string {
		c := exec.Command("go", args...)
		c.Dir, c.Env = dir, env
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	sh("test", "-coverprofile=c.out", "./...")
	total := sh("tool", "cover", "-func=c.out")
	m := regexp.MustCompile(`total:\s+\(statements\)\s+([0-9.]+)%`).FindStringSubmatch(total)
	if m == nil {
		t.Fatalf("no total line in\n%s", total)
	}
	got, err := ReadProfile(filepath.Join(dir, "c.out"), "example.com/fix")
	if err != nil {
		t.Fatal(err)
	}
	if want := m[1]; fmt.Sprintf("%.1f", got["p"].Percent()) != want {
		t.Errorf("covfloor %.1f%% != go tool cover %s%%", got["p"].Percent(), want)
	}
}
