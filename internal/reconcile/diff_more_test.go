package reconcile

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakePEM is a made-up key body: never a real key.
const fakePEM = "-----BEGIN PRIVATE KEY-----\n" +
	"MIIEvQIBADANBgkqhkiG9w0BAQEFAASCFAKEFAKEFAKE\n" +
	"ZZtopsecretbody0123456789abcdefFAKEFAKE==\n" +
	"-----END PRIVATE KEY-----"

// TestUnifiedEnvRedactsMultiLineValues pins that no line of a quoted
// multi-line value (a PEM key) reaches --diff text, including the base64 tail
// line that looks like KEY=.
func TestUnifiedEnvRedactsMultiLineValues(t *testing.T) {
	before := "A=1\nJWT_KEY=\"" + fakePEM + "\"\nTAIL=keep-out-1\n"
	after := "A=2\nJWT_KEY=\"" + strings.ReplaceAll(fakePEM, "FAKEFAKE", "OTHER123") + "\"\nTAIL=keep-out-2\n"
	singleQuoted := "K='line-one-secret\nline-two-secret=\nline-three-secret'\nNEXT=x\n"
	backslash := "LONG=part-one-secret \\\npart-two-secret\nAFTER=y\n"
	for _, c := range []struct{ name, a, b string }{
		{"double-quoted PEM", before, after},
		{"single-quoted", singleQuoted, strings.ReplaceAll(singleQuoted, "NEXT=x", "NEXT=z")},
		{"backslash continuation", backslash, strings.ReplaceAll(backslash, "AFTER=y", "AFTER=z")},
		{"added", "", before},
		{"removed", before, ""},
	} {
		u := Unified(".env.prod", []byte(c.a), []byte(c.b))
		for _, leak := range []string{"MIIEvQ", "ZZtop", "BEGIN PRIVATE", "END PRIVATE", "OTHER123", "secret", "keep-out", "FAKEFAKE"} {
			if strings.Contains(u, leak) {
				t.Errorf("%s: unified text leaks %q:\n%s", c.name, leak, u)
			}
		}
	}
	u := Unified(".env", []byte(before), []byte(after))
	if !strings.Contains(u, "JWT_KEY=[REDACTED]") || !strings.Contains(u, "-[REDACTED]") && !strings.Contains(u, " [REDACTED]") {
		t.Errorf("key name and redaction markers expected:\n%s", u)
	}
	// Lines after the closing quote are ordinary again.
	if !strings.Contains(u, "TAIL=[REDACTED]") || !strings.Contains(u, "A=[REDACTED]") {
		t.Errorf("assignments after a multi-line value must still show their key:\n%s", u)
	}
}

func TestRedactEnvLinesShapes(t *testing.T) {
	in := []string{"# note\n", "\n", "export K=v\n", "stray text\n", "# token=abc\n", "Q=\"one line\"\n", "AFTER=1"}
	want := []string{"# note\n", "\n", "K=[REDACTED]\n", "[REDACTED]\n", "# token=[REDACTED]\n", "Q=[REDACTED]\n", "AFTER=[REDACTED]"}
	got := redactEnvLines(in)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiffAddRemoveOverCapReportMinusOne(t *testing.T) {
	big := strings.Repeat("line\n", MaxDiffLines+1)
	got := Diff(set("gone.txt", big), set("new.txt", big), nil)
	if len(got) != 2 || got[0].Action != ActionRemove || got[0].DiffLines != -1 || got[1].Action != ActionAdd || got[1].DiffLines != -1 {
		t.Errorf("got %+v", got)
	}
	atCap := strings.Repeat("line\n", MaxDiffLines)
	if got := Diff(nil, set("n", atCap), nil); got[0].DiffLines != MaxDiffLines {
		t.Errorf("at the cap the count is exact: %+v", got)
	}
}

// TestDiffWorstCaseIsBounded: two unrelated 20,000-line files (edit distance
// 40,000) report -1 quickly instead of searching to the end.
func TestDiffWorstCaseIsBounded(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i < MaxDiffLines; i++ {
		fmt.Fprintf(&a, "a%d\n", i)
		fmt.Fprintf(&b, "b%d\n", i)
	}
	start := time.Now()
	got := Diff(set("f", a.String()), set("f", b.String()), nil)
	u := Unified("f", []byte(a.String()), []byte(b.String()))
	if got[0].DiffLines != -1 || !strings.Contains(u, "diff too large to display @@") {
		t.Errorf("diff_lines=%d unified=%q", got[0].DiffLines, u)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Errorf("worst case took %s; the bound maxEditDistance=%d should keep it fast", el, maxEditDistance)
	}
}

// TestDiffTrimsCommonEnds: a one-line change in a 20,000-line file is exact and
// its script rebuilds the new text.
func TestDiffTrimsCommonEnds(t *testing.T) {
	base := strings.Repeat("same\n", MaxDiffLines)
	changed := strings.Repeat("same\n", MaxDiffLines/2) + "different\n" + strings.Repeat("same\n", MaxDiffLines/2-1)
	if got := Diff(set("f", base), set("f", changed), nil); got[0].DiffLines != 2 {
		t.Errorf("diff_lines = %d, want 2", got[0].DiffLines)
	}
	u := Unified("f", []byte(base), []byte(changed))
	if !strings.Contains(u, "-same\n+different\n") || !strings.Contains(u, "@@ -9998,7 +9998,7 @@") {
		t.Errorf("unexpected hunk:\n%s", u)
	}
}
