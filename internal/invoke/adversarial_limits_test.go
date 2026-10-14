package invoke

// Purpose: adversarial size, nesting and member-name tests of
// TestInvokeAdversarial (oversized requests, deeply nested JSON, duplicate
// and case-folded members).
// Constraints: a hostile document is refused E420 quickly; the time bound is a
// generous wall-clock guard against a quadratic or stack-hungry decoder, not a
// benchmark.

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nself-org/cli/internal/errs"
)

// decodeFast decodes data and fails the test when it takes longer than 10 s.
func decodeFast(t *testing.T, data string) (Request, error) {
	t.Helper()
	start := time.Now()
	r, err := DecodeRequest([]byte(data))
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("DecodeRequest took %s on %d bytes", d, len(data))
	}
	return r, err
}

// wantE420 asserts err is an E420 CLIError whose text is bounded.
func wantE420(t *testing.T, err error, what string) {
	t.Helper()
	var ce *errs.CLIError
	if err == nil {
		t.Errorf("%s: accepted, want E420", what)
		return
	}
	if !errorsAs(err, &ce) || ce.Code != "E420" {
		t.Errorf("%s: want E420, got %T %v", what, err, err)
		return
	}
	if n := len(ce.What) + len(ce.Why); n > 2048 {
		t.Errorf("%s: the refusal text is %d bytes", what, n)
	}
}

func advSizeLimits(t *testing.T) {
	t.Run("request larger than MaxRequestBytes is refused before parsing", func(t *testing.T) {
		_, err := decodeFast(t, strings.Repeat(" ", MaxRequestBytes)+"{}")
		wantE420(t, err, "MaxRequestBytes+2")
		_, err = decodeFast(t, `{"args":["`+strings.Repeat("a", MaxRequestBytes)+`"]}`)
		wantE420(t, err, "big arg")
		if _, err := decodeFast(t, strings.Repeat(" ", MaxRequestBytes-2)+"{}"); err != nil {
			t.Errorf("a request of exactly MaxRequestBytes is within the limit: %v", err)
		}
	})

	t.Run("one element over MaxElementBytes is refused, nothing runs", func(t *testing.T) {
		e := advFixture(t)
		e.wantRefusal(e.call("fx echo", doc(t, map[string]any{"args": []string{strings.Repeat("a", MaxElementBytes+1)}})), 0, "E420")
		e.wantRefusal(e.call("fx echo", doc(t, map[string]any{"args": []string{"w"}, "flags": map[string]any{"name": strings.Repeat("a", MaxElementBytes)}})), 0, "E420")
		e.wantRefusal(e.call("fx echo", doc(t, map[string]any{"args": []string{"w"}, "flags": map[string]any{"items": []string{"ok", strings.Repeat("a", MaxElementBytes)}}})), 0, "E420")
		e.wantRefusal(e.call("demo", doc(t, map[string]any{"argv": []string{strings.Repeat("a", MaxElementBytes+1)}})), 0, "E420")
		// the byte limit counts bytes, not runes
		e.wantRefusal(e.call("fx echo", doc(t, map[string]any{"args": []string{strings.Repeat("\u20ac", MaxElementBytes/3+1)}})), 0, "E420")
	})

	t.Run("an element of exactly MaxElementBytes still runs", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("a Windows command line is capped at 32767 characters, below MaxElementBytes plus the rest of the argv")
		}
		e := advFixture(t)
		got := echoedArgv(t, e.call("fx echo", doc(t, map[string]any{"args": []string{strings.Repeat("a", MaxElementBytes)}})))
		if len(got) != 5 || len(got[4]) != MaxElementBytes {
			t.Errorf("argv shape %d elements", len(got))
		}
	})

	t.Run("the whole argv is bounded by MaxArgvBytes", func(t *testing.T) {
		e := advFixture(t)
		chunk := strings.Repeat("a", 30000)
		e.wantRefusal(e.call("fx echo", doc(t, map[string]any{"args": []string{chunk, chunk, chunk, chunk, chunk}})), 0, "E420")
		// Element-count limits are checked on the builder directly: a refused
		// 70000-arg request costs seconds of audit redaction under -race (finding A2).
		many := make([]string, 70000)
		for i := range many {
			many[i] = "a"
		}
		for name, r := range map[string]Request{
			"args":      {Args: many},
			"flag item": {Args: []string{"w"}, Flags: map[string]any{"items": many}},
		} {
			_, err := BuildArgv(mustCmd(t, e.reg, "fx echo"), r)
			wantE420(t, err, name)
		}
		_, err := BuildArgv(mustCmd(t, e.reg, "demo"), Request{Argv: many})
		wantE420(t, err, "free-form argv")
	})

	t.Run("argument counts are enforced against the declared args", func(t *testing.T) {
		e := advGate(t)
		e.wantRefusal(e.call("gx put", `{"args":["k","v","extra"]}`), 0, "E420")
		e.wantRefusal(e.call("gx put", `{"args":["k"]}`), 0, "E420")
		e.wantRefusal(e.call("gx put", `{}`), 0, "E420")
		e.wantRefusal(e.call("gx read", `{"args":["x"]}`), 0, "E420")
		// a refused request with too many args never issues a nonce for a destructive command
		e.wantRefusal(e.call("gx wipe", `{"args":["a","b"]}`), 0, "E420")
		if n := e.guard.liveNonces(); n != 0 {
			t.Errorf("a request that cannot run was given %d nonces", n)
		}
	})

	t.Run("a huge flag object is refused with a bounded envelope", func(t *testing.T) {
		e := advFixture(t)
		var b strings.Builder
		b.WriteString(`{"args":["w"],"flags":{`)
		for i := 0; i < 3000; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`"f` + strings.Repeat("x", i%7) + strconv.Itoa(i) + `":1`)
		}
		b.WriteString(`}}`)
		res := e.call("fx echo", b.String())
		e.wantRefusal(res, 0, "E420")
		if len(res.Stdout) > 4096 {
			t.Errorf("the envelope is %d bytes", len(res.Stdout))
		}
	})

	t.Run("an unknown command path of any length is echoed bounded", func(t *testing.T) {
		e := advFixture(t)
		res := e.call(strings.Repeat("z", 200000), `{}`)
		e.wantRefusal(res, 0, "E432")
		if len(res.Stdout) > 2048 {
			t.Errorf("the E432 envelope is %d bytes", len(res.Stdout))
		}
	})
}

// advDeepNesting: deeply nested bodies are E420 quickly, no stack blow up.
func advDeepNesting(t *testing.T) {
	const deep = 400000 // 2 * 400000 bytes stays under MaxRequestBytes
	cases := map[string]string{
		"flag value nested arrays":  `{"flags":{"name":` + repeatJSON("[", "1", "]", deep) + `}}`,
		"flag value nested objects": `{"flags":{"name":` + repeatJSON(`{"a":`, "1", "}", deep/3) + `}}`,
		"args nested arrays":        `{"args":` + repeatJSON("[", "", "]", deep) + `}`,
		"argv nested arrays":        `{"argv":` + repeatJSON("[", "", "]", deep) + `}`,
		"unknown member nested":     `{"x":` + repeatJSON("[", "", "]", deep) + `}`,
		"flags object nested":       `{"flags":` + repeatJSON(`{"a":`, "1", "}", deep/3) + `}`,
		"nested then duplicate":     `{"flags":{"a":` + repeatJSON("[", "", "]", deep) + `,"a":1}}`,
		"unterminated nesting":      `{"flags":{"name":` + strings.Repeat("[", deep),
		"confirm nested":            `{"confirm":` + repeatJSON("[", "", "]", deep) + `}`,
		"just over the json depth":  `{"flags":{"name":` + repeatJSON("[", "1", "]", 10001) + `}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := decodeFast(t, body)
			wantE420(t, err, name)
		})
	}

	t.Run("a nested flag value that decodes is refused by the argv builder", func(t *testing.T) {
		e := advFixture(t)
		for _, v := range []string{repeatJSON("[", `"x"`, "]", 64), repeatJSON(`{"a":`, `"x"`, "}", 64), `[{"a":["b"]}]`, `[[]]`, repeatJSON("[", "1", "]", 9990)} {
			e.wantRefusal(e.call("fx echo", `{"args":["w"],"flags":{"tags":`+v+`}}`), 0, "E420")
			e.wantRefusal(e.call("fx echo", `{"args":["w"],"flags":{"name":`+v+`}}`), 0, "E420")
		}
	})

	t.Run("nested gated request never issues a nonce", func(t *testing.T) {
		e := advGate(t)
		e.wantRefusal(e.call("gx wipe", `{"flags":{"target":`+repeatJSON("[", `"x"`, "]", 200)+`}}`), 0, "E420")
		if n := e.guard.liveNonces(); n != 0 {
			t.Errorf("%d nonces issued", n)
		}
	})
}

// advMembers: member names must be exact and appear once.
func advMembers(t *testing.T) {
	h := strings.Repeat("a", 64)
	g := strings.Repeat("b", 64)
	bad := map[string]string{
		"duplicate args":               `{"args":["a"],"args":["b"]}`,
		"case-folded args":             `{"args":["a"],"ARGS":["b"]}`,
		"only a folded member":         `{"Args":["a"]}`,
		"folded flags":                 `{"FLAGS":{}}`,
		"folded confirm":               `{"Confirm":"` + h + `"}`,
		"unicode long s":               "{\"arg\u017f\":[\"x\"]}",
		"kelvin sign member":           "{\"\u212a\":1}",
		"escaped duplicate":            `{"ar\u0067s":["a"],"args":["b"]}`,
		"duplicate flag":               `{"flags":{"name":"a","name":"b"}}`,
		"escaped duplicate flag":       `{"flags":{"name":"a","n\u0061me":"b"}}`,
		"duplicate flags object":       `{"flags":{"a":1},"flags":{"b":2}}`,
		"duplicate argv":               `{"argv":[],"argv":[]}`,
		"duplicate confirm":            `{"confirm":"` + h + `","confirm":"` + g + `"}`,
		"empty member name":            `{"":1}`,
		"proto member":                 `{"__proto__":{}}`,
		"two objects":                  `{"args":["a"]}{"args":["b"]}`,
		"object then array":            `{} []`,
		"BOM prefix":                   "\ufeff{}",
		"vertical tab prefix":          "\v{}",
		"form feed suffix":             "{}\f",
		"nbsp prefix":                  "\u00a0{}",
		"trailing comma":               `{"args":["a"],}`,
		"single quotes":                `{'args':[]}`,
		"line comment":                 "{\"args\":[]} // x",
		"block comment":                `{/*x*/"args":[]}`,
		"unquoted member":              `{args:[]}`,
		"nan literal":                  `{"flags":{"count":NaN}}`,
		"leading zero number":          `{"flags":{"count":007}}`,
		"hex number":                   `{"flags":{"count":0x10}}`,
		"member with NUL in name":      "{\"args\\u0000\":[]}",
		"flags as array":               `{"flags":[]}`,
		"flags as string":              `{"flags":"x"}`,
		"args as object":               `{"args":{}}`,
		"args with null element":       `{"args":[null]}`,
		"argv with number":             `{"argv":[1]}`,
		"confirm as number":            `{"confirm":1}`,
		"confirm not lower hex":        `{"confirm":"` + strings.ToUpper(h) + `"}`,
		"top level string":             `"x"`,
		"top level number":             `1`,
		"empty array":                  `[]`,
		"literal null":                 `null`,
		"member after embedded object": `{"flags":{"a":1},"extra":{"args":[]}}`,
	}
	t.Run("hostile documents are E420", func(t *testing.T) {
		for name, body := range bad {
			_, err := decodeFast(t, body)
			wantE420(t, err, name)
		}
	})

	t.Run("exact members decode as written", func(t *testing.T) {
		r, err := DecodeRequest([]byte(`{"args":["safe"],"flags":{"name":"v","Name":"w"},"argv":["a"],"confirm":"` + h + `"}`))
		if err != nil || len(r.Args) != 1 || r.Args[0] != "safe" || r.Flags["name"] != "v" || r.Flags["Name"] != "w" || r.Confirm != h {
			t.Errorf("decode: %+v %v", r, err)
		}
	})

	t.Run("flag names that differ only by case are different names, never folded", func(t *testing.T) {
		e := advFixture(t)
		e.wantRefusal(e.call("fx echo", `{"args":["w"],"flags":{"name":"a","NAME":"b"}}`), 0, "E420")
		e.wantRefusal(e.call("fx echo", `{"args":["w"],"flags":{"Name":"a"}}`), 0, "E420")
	})

	t.Run("invalid UTF-8 and lone surrogates never reach argv as raw bytes", func(t *testing.T) {
		e := advFixture(t)
		raw := "{\"args\":[\"a\xffb\",\"\\ud800x\"]}"
		r, err := DecodeRequest([]byte(raw))
		if err != nil {
			wantE420(t, err, "invalid utf-8")
			return
		}
		for _, a := range r.Args {
			if !utf8.ValidString(a) {
				t.Errorf("decoded arg %q is not valid UTF-8", a)
			}
		}
		for _, a := range echoedArgv(t, e.call("fx echo", raw)) {
			if !utf8.ValidString(a) {
				t.Errorf("argv element %q is not valid UTF-8", a)
			}
		}
	})
}
