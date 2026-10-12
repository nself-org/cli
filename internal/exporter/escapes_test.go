package exporter

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

// The renderings below are written out by hand, the way Postgres prints each
// type and the way pg_restore then prints it inside COPY text or an SQL
// literal. They deliberately do not call the escape helpers under test.
// The fake secret holds a quote, a backslash and punctuation.
const escSecret = `pa"ss\wd!x9`

func TestScrubContainerRenderings(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"raw copy row", "1\tpa\"ss\\\\wd!x9\n"},
		{"jsonb in COPY", `{"smtp": "pa\\"ss\\\\wd!x9"}`},
		{"json in COPY", `{"smtp":"pa\\"ss\\\\wd!x9"}`},
		{"text[] in COPY", `{"pa\\"ss\\\\wd!x9"}`},
		{"text[] with sibling in COPY", `{"alpha","pa\\"ss\\\\wd!x9"}`},
		{"hstore in COPY", `"smtp"=>"pa\\"ss\\\\wd!x9"`},
		{"composite in COPY", `("pa""ss\\\\wd!x9",1)`},
		{"jsonb[] in COPY", `{"{\\"k\\": \\"pa\\\\\\"ss\\\\\\\\wd!x9\\"}"}`},
		{"jsonb in COMMENT literal", `COMMENT ON TABLE t IS '{"k": "pa\"ss\\wd!x9"}';`},
		{"jsonb default", `ALTER TABLE t ALTER COLUMN c SET DEFAULT '{"k": "pa\"ss\\wd!x9"}'::jsonb;`},
		{"text[] default", `DEFAULT '{"pa\"ss\\wd!x9"}'::text[]`},
		{"raw comment", `COMMENT ON TABLE t IS 'pa"ss\wd!x9';`},
	} {
		hit, err := scan(strings.NewReader("prefix "+tc.text+" suffix"), secretPatterns([]string{escSecret}))
		if !hit || err != nil {
			t.Errorf("%s: secret missed (%v)", tc.name, err)
		}
	}
}

func TestScrubContainerQuotesInSecret(t *testing.T) {
	// A single quote adds the SQL literal doubling on top of each container.
	secret := `it's "x"\y`
	for _, text := range []string{
		`COMMENT ON TABLE t IS 'it''s "x"\y';`,
		`COMMENT ON TABLE t IS '{"k": "it''s \"x\"\\y"}';`,
		`{"k": "it's \\"x\\"\\\\y"}`,
		`{"it's \\"x\\"\\\\y"}`,
		`("it's ""x""\\\\y")`,
	} {
		if hit, err := scan(strings.NewReader(text), secretPatterns([]string{secret})); !hit || err != nil {
			t.Errorf("secret missed in %s (%v)", text, err)
		}
	}
	// Control characters: jsonb \n, COPY doubles the backslash.
	for _, text := range []string{`{"k": "line1\\nline2"}`, "line1\\nline2", `{"k": "line1\\tline2"}`} {
		if hit, _ := scan(strings.NewReader(text), secretPatterns([]string{"line1\nline2", "line1\tline2"})); !hit {
			t.Errorf("control-character secret missed in %s", text)
		}
	}
}

func TestScrubContainerNoFalsePositive(t *testing.T) {
	for _, text := range []string{`{"k": "unrelated"}`, `{"pa\\"ss"}`, "1\tpa\"ss\n"} {
		if hit, _ := scan(strings.NewReader(text), secretPatterns([]string{escSecret})); hit {
			t.Errorf("false positive on %s", text)
		}
	}
}

func TestSecretFormsAreDistinctAndBounded(t *testing.T) {
	if got := secretForms("plainvalue123"); len(got) != 1 {
		t.Fatalf("ordinary secret produced %d forms", len(got))
	}
	seen := map[string]bool{}
	forms := secretForms(escSecret)
	for _, f := range forms {
		if seen[f] || f == "" {
			t.Fatalf("duplicate or empty form %q", f)
		}
		seen[f] = true
	}
	if len(forms) > 160 {
		t.Fatalf("%d forms", len(forms))
	}
}

// Client-chosen JSON escapes are stored verbatim by json and text columns.
// Expected renderings are written by hand; ab/c+d-E9 is the fake secret.
func TestScrubClientJSONEscapes(t *testing.T) {
	secret := "ab/c+d-E9"
	for _, tc := range []struct{ name, text string }{
		{"php slash", `{"k":"ab\/c+d-E9"}`},
		{"php slash in COPY", `{"k":"ab\\/c+d-E9"}`},
		{"lower unicode", `{"k":"ab\u002fc\u002bd\u002dE9"}`},
		{"upper unicode", `{"k":"ab\u002Fc\u002Bd\u002DE9"}`},
		{"lower unicode in COPY", `{"k":"ab\\u002fc\\u002bd\\u002dE9"}`},
		{"upper unicode in SQL literal", `COMMENT ON TABLE t IS '{"k":"ab\u002Fc\u002Bd\u002DE9"}';`},
	} {
		if hit, err := scan(strings.NewReader("x "+tc.text+" y"), secretPatterns([]string{secret})); !hit || err != nil {
			t.Errorf("%s: secret missed (%v)", tc.name, err)
		}
	}
	// A quote and a slash together: PHP writes \" and \/.
	if hit, _ := scan(strings.NewReader(`{"k":"a\"b\/c9x"}`), secretPatterns([]string{`a"b/c9x`})); !hit {
		t.Error("quote plus slash secret missed")
	}
}

func TestClientJSONFormsExact(t *testing.T) {
	want := []string{
		`ab\/c`, `ab\u002fc`, `ab\u002Fc`,
		`ab\\/c`, `ab\\u002fc`, `ab\\u002Fc`,
	}
	got := map[string]bool{}
	for _, f := range clientJSONForms("ab/c") {
		got[f] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing form %q", w)
		}
	}
	if len(got) > 12 {
		t.Errorf("%d forms for a short secret", len(got))
	}
	if f := clientJSONForms("plainvalue123"); len(f) != 0 {
		t.Errorf("alphanumeric secret produced %v", f)
	}
}

func TestClientJSONFormsNoFalsePositive(t *testing.T) {
	for _, text := range []string{`{"url":"https://example.invalid/a/b"}`, `{"k":"ab/c"}`, `{"k":"ab\u0041c"}`, "see /etc/hosts and a+b"} {
		if hit, _ := scan(strings.NewReader(text), secretPatterns([]string{"ab/c+d-E9"})); hit {
			t.Errorf("false positive on %s", text)
		}
	}
}

func TestJSONTextMatchesPostgres(t *testing.T) {
	// jsonb_out leaves <, >, & and non-ASCII alone and escapes control bytes.
	if got := jsonText("a<b>&é\x01\x1f\b\f"); got != `a<b>&`+"é"+`\u0001\u001f\b\f` {
		t.Fatalf("%q", got)
	}
}

func TestRemoveScanFailureFailsExport(t *testing.T) {
	fakeTools(t, "plain sql without secrets", false)
	old := removeScan
	removeScan = func(string) error { return errors.New("fake removal failure") }
	t.Cleanup(func() { removeScan = old })
	dir := filepath.Join(t.TempDir(), "bundle")
	w, err := portable.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.WriteFile("db/data.dump", strings.NewReader("fake opaque dump")); err != nil {
		t.Fatal(err)
	}
	m, err := w.Finish(portable.Manifest{})
	if err != nil {
		t.Fatal(err)
	}
	err = scrubBundle(context.Background(), fakeConfig(), dir, m, []string{"FAKE-EXPORT-SECRET-ONLY"})
	if err == nil || !strings.Contains(err.Error(), "remove SQL scan") {
		t.Fatalf("failed removal ignored: %v", err)
	}
}
