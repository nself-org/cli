package invoke

// Purpose: adversarial path-traversal and control-character tests of
// TestInvokeAdversarial.
// Constraints: the invoker never interprets a path; it moves a value to the
// child as exactly one argv element, except a NUL byte, which no argv can hold
// and is E420.

import (
	"reflect"
	"strings"
	"testing"
)

func advControlChars(t *testing.T) {
	verbatim := []string{
		"../../etc/passwd", "/etc/shadow", `..\..\windows\system32`, "file:///etc/passwd", "~/.ssh/id_rsa", "$HOME/.ssh",
		"a\nb", "a\rb", "a\r\nb", "\x1b[2J", "a\tb", "\u202e", "\u2028", "\u2029", "%00", "%2e%2e/", "a\\u0000b", "", " ", "\x7f", "\x01\x02\x1f",
		"..", ".", "/", "//", "C:\\", "\u00e9\u0301", "\U0001f4a9", "-", "--", "a b", "a=b", "=a",
	}
	t.Run("traversal and control characters in args are one element each", func(t *testing.T) {
		e := advFixture(t)
		got := echoedArgv(t, e.call("fx echo", doc(t, map[string]any{"args": verbatim})))
		want := append([]string{"fx", "echo", "--json", "--"}, verbatim...)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("argv %q\nwant %q", got, want)
		}
	})

	t.Run("traversal and control characters in flag values are one element each", func(t *testing.T) {
		for _, v := range verbatim {
			e := advFixture(t)
			got := echoedArgv(t, e.call("fx echo", doc(t, map[string]any{"args": []string{"w"}, "flags": map[string]any{"name": v, "items": []string{v}}})))
			want := []string{"fx", "echo", "--json", "--items=" + v, "--name=" + v, "--", "w"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("value %q: argv %q, want %q", v, got, want)
			}
		}
	})

	t.Run("a NUL byte is E420 wherever it sits", func(t *testing.T) {
		e := advFixture(t)
		for name, body := range map[string]string{
			"arg":                `{"args":["a\u0000b"]}`,
			"arg first byte":     `{"args":["\u0000"]}`,
			"flag value":         `{"args":["w"],"flags":{"name":"a\u0000"}}`,
			"flag name":          `{"args":["w"],"flags":{"na\u0000me":"a"}}`,
			"slice item":         `{"args":["w"],"flags":{"items":["ok","\u0000"]}}`,
			"secret flag value":  `{"args":["w"],"flags":{"tok":"nself_test_FAKE_\u0000_0001"}}`,
			"duration":           `{"args":["w"],"flags":{"wait":"1s\u0000"}}`,
			"free-form argv":     `{"argv":["x\u0000"]}`,
			"free-form after --": `{"argv":["--","x\u0000"]}`,
		} {
			path := "fx echo"
			if strings.Contains(name, "free-form") {
				path = "demo"
			}
			if res := e.call(path, body); errorOf(t, res)["code"] != "E420" {
				t.Errorf("%s: %s", name, res.Stdout)
			}
		}
		g := advGate(t)
		g.wantRefusal(g.call("gx put", `{"args":["k","sec\u0000ret"]}`), 0, "E420")
		g.wantRefusal(g.call("gx wipe", `{"args":["a\u0000"]}`), 0, "E420")
		if n := g.guard.liveNonces(); n != 0 {
			t.Errorf("a NUL request was given %d nonces", n)
		}
		if len(e.argvs()) != 0 {
			t.Errorf("a child ran: %v", e.argvs())
		}
	})

	t.Run("a list flag refuses the characters its CSV split would reinterpret", func(t *testing.T) {
		e := advFixture(t)
		for _, item := range []string{"a,b", "a\nb", "a\rb", "a\"b", ""} {
			e.wantRefusal(e.call("fx echo", doc(t, map[string]any{"args": []string{"w"}, "flags": map[string]any{"tags": []string{"ok", item}}})), 0, "E420")
		}
	})

	t.Run("a hostile command path never starts a child or leaks to argv", func(t *testing.T) {
		e := advFixture(t)
		for _, p := range []string{"../../bin/sh", "/bin/sh", "fx echo\x00", "fx echo\n--yes", "fx\techo ../x", "\x1b[2J", "fx echo \u202e"} {
			res := e.call(p, `{"args":["w"]}`)
			// a path that only differs by whitespace may resolve to the same registry command: then it ran fx echo and nothing else
			if res.ErrorCode == "" {
				if got := echoedArgv(t, res); !reflect.DeepEqual(got[:2], []string{"fx", "echo"}) {
					t.Errorf("path %q ran %q", p, got)
				}
				continue
			}
			if res.ErrorCode != "E432" {
				t.Errorf("path %q: %s", p, res.Stdout)
			}
		}
		for _, av := range e.argvs() {
			if av[0] != "fx" || av[1] != "echo" {
				t.Errorf("a hostile path started %q", av)
			}
		}
	})
}
