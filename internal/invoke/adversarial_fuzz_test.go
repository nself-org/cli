package invoke

// Purpose: FuzzDecodeRequest proves DecodeRequest is total and exact for
// arbitrary bytes.
// Property: it never panics; a refusal is always an E420 *errs.CLIError whose
// text is bounded; an accepted request is no larger than MaxRequestBytes, has
// a confirm that is empty or 64 lowercase hex, and equals an exact reading of
// the bytes (exactMembers: no folded or repeated member).
// Constraints: seeds also live in testdata/fuzz/FuzzDecodeRequest.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func FuzzDecodeRequest(f *testing.F) {
	hex := strings.Repeat("ab", 32)
	for _, s := range []string{
		``, `{}`, ` {} `, `{"args":["a"]}`, `{"args":["--json","-x","--"],"flags":{"name":"--yes=true"}}`,
		`{"flags":{"tags":["a","b"],"on":true,"count":1,"ratio":1.5e3}}`, `{"argv":["--","x"]}`, `{"confirm":"` + hex + `"}`,
		`{"ARGS":["a"]}`, `{"args":["a"],"args":["b"]}`, `{"flags":{"x":1,"x":2}}`, "{\"arg\u017f\":[]}", `{"args":["a"],"args":["b"]}`,
		`{"args":["a\u0000b"]}`, `{"confirm":"` + strings.ToUpper(hex) + `"}`, "\ufeff{}", "\v{}", `{} {}`, `[]`, `null`,
		`{"flags":{"name":` + strings.Repeat("[", 2000) + strings.Repeat("]", 2000) + `}}`, `{"args":[` + strings.Repeat(`"a",`, 50) + `"a"]}`,
		`{"flags":{"n":1e999999}}`, "{\"args\":[\"\xff\"]}", `{"args":["\ud800"]}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := DecodeRequest(data)
		if err != nil {
			var ce *errs.CLIError
			if !errorsAs(err, &ce) || ce.Code != "E420" {
				t.Fatalf("a refusal must be an E420 CLIError, got %T %v", err, err)
			}
			if n := len(ce.What) + len(ce.Why); n > 4096 {
				t.Fatalf("the refusal text is %d bytes", n)
			}
			return
		}
		if len(data) > MaxRequestBytes {
			t.Fatalf("accepted %d bytes, limit %d", len(data), MaxRequestBytes)
		}
		if r.Confirm != "" && !confirmPattern.MatchString(r.Confirm) {
			t.Fatalf("accepted confirm %q", r.Confirm)
		}
		exactMembers(t, data, r)
	})
}
