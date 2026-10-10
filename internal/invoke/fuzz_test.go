package invoke

// Purpose: FuzzBuildArgv proves the argv layout property for arbitrary input.
// Property: the argv is <path tokens> --json <flag elements> [-- <args>]; before
// the first "--" only the path tokens, "--json" and flags the request named
// with exposed names appear; after it exactly the request args; no element has
// a NUL; every failure is an E420 CLIError (never a panic or another error).
// Constraints: seeds also live in testdata/fuzz/FuzzBuildArgv.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func FuzzBuildArgv(f *testing.F) {
	reg := fixtureRegistry(f)
	cmd := mustCmd(f, reg, "fx echo")
	exposed := map[string]string{}
	var names []string
	for _, fl := range ExposedFlags(cmd, TransportMCP) {
		exposed[fl.Name] = fl.Type
		names = append(names, fl.Name)
	}
	path := []string{"fx", "echo"}

	for _, s := range []struct {
		a0, a1, fname, fval string
		sel                 byte
	}{
		{"word", "more", "name", "plain", 0},
		{"--json", "-x", "name", "--tok=evil", 1},
		{"a b", "l1\nl2", "tags", "a,b", 2},
		{"$(id)", ";rm -rf /", "items", "`id`", 3},
		{"w", "--", "on", "true", 4},
		{"w", "", "json", "x", 250},
		{"w\x00", "", "name", "v\x00", 1},
		{"w", "", "count", "1.5", 5},
	} {
		f.Add(s.a0, s.a1, s.fname, s.fval, s.sel)
	}

	f.Fuzz(func(t *testing.T, a0, a1, fname, fval string, sel byte) {
		pool := append(append([]string{}, names...), fname)
		name := pool[int(sel)%len(pool)]
		var value any
		switch sel % 6 {
		case 0:
			value = fval
		case 1:
			value = true
		case 2:
			value = []string{fval, a1}
		case 3:
			value = float64(len(fval))
		case 4:
			value = []any{fval}
		default:
			value = false
		}
		r := Request{Args: []string{a0, a1}, Flags: map[string]any{name: value}}
		argv, err := BuildArgv(cmd, r)
		if err != nil {
			var ce *errs.CLIError
			if !errorsAs(err, &ce) || ce.Code != "E420" {
				t.Fatalf("a refusal must be E420, got %T %v", err, err)
			}
			return
		}
		for _, el := range argv {
			if strings.IndexByte(el, 0) >= 0 {
				t.Fatalf("NUL in %q", argv)
			}
		}
		if len(argv) < 3 || argv[0] != path[0] || argv[1] != path[1] || argv[2] != "--json" {
			t.Fatalf("bad head %q", argv)
		}
		sep := -1
		for i := 3; i < len(argv); i++ {
			if argv[i] == "--" {
				sep = i
				break
			}
		}
		flagEnd := len(argv)
		if sep >= 0 {
			flagEnd = sep
		}
		for _, el := range argv[3:flagEnd] {
			if !strings.HasPrefix(el, "--") {
				t.Fatalf("non-flag element %q before --: %q", el, argv)
			}
			got, _, _ := strings.Cut(el[2:], "=")
			if _, ok := exposed[got]; !ok || got != name {
				t.Fatalf("flag %q was not named by the request with an exposed name: %q", got, argv)
			}
		}
		if sep < 0 {
			t.Fatalf("two args were requested but argv has no --: %q", argv)
		}
		after := argv[sep+1:]
		if len(after) != 2 || after[0] != a0 || after[1] != a1 {
			t.Fatalf("after -- got %q, want exactly the request args %q", after, r.Args)
		}
	})
}
