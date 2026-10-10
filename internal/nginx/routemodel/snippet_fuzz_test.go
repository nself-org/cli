package routemodel

// snippet_fuzz_test.go — fuzzing for the snippet reader. Plugin snippets are
// third-party input, so the reader must never panic, hang, or lose a token.
//
// Seed corpus: the 55 free-plugin snippets (testdata/snippets) rendered the
// way the build renders them, plus a few hostile shapes.

import (
	"strings"
	"testing"
	"time"
)

// scanWords splits text like the tokenizer should, without sharing its code:
// words end at whitespace or ; { }, a # that begins a word starts a comment.
func scanWords(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		cur := ""
		flush := func() {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		}
	scan:
		for i := 0; i < len(line); i++ {
			switch c := line[i]; {
			case strings.IndexByte(" \t\r\f\v;{}", c) >= 0:
				flush()
			case c == '#' && cur == "":
				break scan
			default:
				cur += line[i : i+1]
			}
		}
		flush()
	}
	return out
}

func FuzzSnippet(f *testing.F) {
	for _, c := range corpusFiles() {
		f.Add(c)
	}
	for _, s := range []string{"server{listen{}}", "", "{", "}", ";", "server{", "server {{{{ location / { proxy_pass $v; set $v http://a:1; }", "a \"b", "${", "$ {x}",
		"upstream u { server a:1; } server { location / { proxy_pass http://u; } } upstream u { server b:2; }"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		start := time.Now()
		sn, err := ParseSnippet("plugin", "p", "f.conf", text)
		if d := time.Since(start); d > time.Second {
			t.Fatalf("took %v", d)
		}
		if err != nil {
			return
		}
		var all []string
		for _, r := range sn.Routes {
			all = append(all, r.Unmodelled...)
		}
		accounted(t, "fuzz", sn, sn.tree, strings.Join(append(all, sn.Global...), "\n"))
		if !strings.ContainsAny(text, "\"'$\\") {
			var got []string
			flatten(sn.tree, &got)
			if want := scanWords(text); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
				t.Fatalf("tokens differ:\n got %q\nwant %q", got, want)
			}
		}
		for _, b := range sn.tree {
			if r1 := b.Render(); r1 != "" {
				again := Tokenize(r1)
				if len(again) != 1 || again[0].Render() != r1 {
					t.Fatalf("render does not round-trip: %q", r1)
				}
			}
		}
	})
}
