package invoke

// Purpose: FuzzBuildArgv proves the argv layout property for arbitrary input.
// Property: the argv is <path tokens> --json <flag elements> [-- <args>]; before
// the first "--" only the path tokens, "--json" and flags the request named
// with exposed names appear; after it exactly the request args; no element has
// a NUL; every failure is an E420 CLIError (never a panic or another error).
// Constraints: seeds also live in testdata/fuzz/FuzzBuildArgv.

import (
	"encoding/json"
	"reflect"
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

// FuzzInvokeRequest drives raw request bytes through DecodeRequest, Exposure,
// CheckFreeForm and BuildArgvFor over the fixture registry, which includes
// plugin parents with destructive and cli-only children (review finding S3).
// Properties of an accepted request:
//  1. what the typed decode holds is what an exact reading of the bytes holds
//     (no folded or repeated member);
//  2. the node cobra resolves for the built argv is the requested command, and
//     that command passes Exposure on the same transport;
//  3. a refusal is E420.
func FuzzInvokeRequest(f *testing.F) {
	reg, tree := fixtureRegistryAndTree(f)
	var paths []string
	for _, c := range reg.Commands {
		paths = append(paths, barePath(c.Path))
	}
	paths = append(paths, "no such command", "")
	transports := []string{TransportMCP, TransportHTTP, TransportHTTPStream}
	for _, doc := range []string{
		`{}`, `{"args":["w"]}`, `{"args":["w"],"flags":{"name":"v","on":true,"tags":["a","b"]}}`,
		`{"argv":["purge","--all"]}`, `{"argv":["leak"]}`, `{"argv":["status","purge"]}`, `{"argv":["--no-monorepo","x"]}`,
		`{"ARGS":["a"]}`, `{"args":["safe"],"ARGS":["evil"]}`, `{"flags":{"x":1,"x":2}}`,
		`{"args":["w"],"flags":{"follow":true}}`, `{"confirm":"` + strings.Repeat("a", 64) + `"}`,
	} {
		for sel := range paths {
			f.Add([]byte(doc), uint8(sel), uint8(sel))
		}
	}
	f.Fuzz(func(t *testing.T, raw []byte, sel, tsel uint8) {
		path := paths[int(sel)%len(paths)]
		transport := transports[int(tsel)%len(transports)]
		r, err := DecodeRequest(raw)
		if err != nil {
			var ce *errs.CLIError
			if !errorsAs(err, &ce) || ce.Code != "E420" {
				t.Fatalf("a decode refusal must be E420, got %T %v", err, err)
			}
			return
		}
		exactMembers(t, raw, r)
		cmd, ok := reg.Lookup(path)
		if !ok {
			return
		}
		if ok, _ := Exposure(cmd, transport, SetFlags(r)); !ok {
			return
		}
		if err := CheckFreeForm(reg, cmd, r); err != nil {
			return
		}
		argv, err := BuildArgvFor(cmd, r, transport)
		if err != nil {
			var ce *errs.CLIError
			if !errorsAs(err, &ce) || ce.Code != "E420" {
				t.Fatalf("an argv refusal must be E420, got %T %v", err, err)
			}
			return
		}
		found, _, ferr := tree.Find(argv)
		if ferr != nil {
			t.Fatalf("an accepted request does not resolve: %v (%q)", ferr, argv)
		}
		got := barePath(found.CommandPath())
		if got != barePath(cmd.Path) {
			t.Fatalf("request for %q resolves to %q in the child: argv %q", cmd.Path, got, argv)
		}
		resolved, ok := reg.Lookup(got)
		if !ok {
			t.Fatalf("resolved %q is not in the registry", got)
		}
		if ok, why := Exposure(resolved, transport, SetFlags(r)); !ok {
			t.Fatalf("resolved %q is not exposed: %s (argv %q)", got, why, argv)
		}
	})
}

// exactMembers asserts the typed decode r equals an exact-name reading of raw.
func exactMembers(t *testing.T, raw []byte, r Request) {
	t.Helper()
	m := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("accepted bytes are not an object: %v", err)
	}
	var exact Request
	for name, v := range m {
		var err error
		switch name {
		case "args":
			err = json.Unmarshal(v, &exact.Args)
		case "argv":
			err = json.Unmarshal(v, &exact.Argv)
		case "confirm":
			err = json.Unmarshal(v, &exact.Confirm)
		case "flags":
			dec := json.NewDecoder(strings.NewReader(string(v)))
			dec.UseNumber()
			err = dec.Decode(&exact.Flags)
		default:
			t.Fatalf("accepted a member named %q", name)
		}
		if err != nil {
			t.Fatalf("member %q: %v", name, err)
		}
	}
	if !reflect.DeepEqual(exact, r) && !(len(exact.Args) == 0 && len(r.Args) == 0 && len(exact.Argv) == 0 && len(r.Argv) == 0 &&
		len(exact.Flags) == 0 && len(r.Flags) == 0 && exact.Confirm == r.Confirm) {
		t.Fatalf("typed decode %+v differs from the exact reading %+v of %q", r, exact, raw)
	}
}
