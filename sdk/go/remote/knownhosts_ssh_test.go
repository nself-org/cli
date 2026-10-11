package remote

// Purpose: tests that the pinned known_hosts store reads lines the way ssh
// does (D-0249 round 2): case-insensitive host fields, the ' '/'\t'-only
// tokenizer, control and non-ASCII bytes, CRLF, and a leading '@' in an
// alias. No network; synthetic keys only.

import (
	"os"
	"strings"
	"testing"
)

func TestCase_AddWritesLowercaseAndConflictsAcrossCase(t *testing.T) {
	p, _ := pinFile(t)
	if err := p.Add("MyAlias", keyA); err != nil {
		t.Fatal(err)
	}
	if got := readPin(t, p); got != "myalias "+keyA+"\n" {
		t.Fatalf("file = %q", got)
	}
	err := p.Add("myalias", keyB) // same type, other key
	if err == nil || !strings.Contains(err.Error(), "already pinned") {
		t.Fatalf("case variant accepted a second key: %v", err)
	}
	if err := p.Add("MYALIAS", keyA); err != nil { // same key: no-op
		t.Fatal(err)
	}
	if got := readPin(t, p); got != "myalias "+keyA+"\n" {
		t.Fatalf("file after no-op = %q", got)
	}
}

func TestCase_HandWrittenUppercaseLine(t *testing.T) {
	for _, op := range []string{"Lookup", "Remove", "Replace", "Add"} {
		t.Run(op, func(t *testing.T) {
			p, _ := pinFile(t)
			writePin(t, p, "MYALIAS "+keyA+"\nother "+keyR+"\n")
			switch op {
			case "Lookup":
				if got, err := p.Lookup("myalias"); err != nil || len(got) != 1 || got[0] != keyA {
					t.Fatalf("Lookup = %q, %v", got, err)
				}
				if got, _ := p.Lookup("MyAlias"); len(got) != 1 {
					t.Fatalf("Lookup(MyAlias) = %q", got)
				}
			case "Remove":
				if err := p.Remove("MyAlias"); err != nil {
					t.Fatal(err)
				}
				if got := readPin(t, p); got != "other "+keyR+"\n" {
					t.Fatalf("file = %q", got)
				}
			case "Replace":
				if err := p.Replace("MyAlias", keyB); err != nil {
					t.Fatal(err)
				}
				if got := readPin(t, p); got != "other "+keyR+"\nmyalias "+keyB+"\n" {
					t.Fatalf("file = %q", got)
				}
			case "Add":
				if err := p.Add("myalias", keyB); err == nil || !strings.Contains(err.Error(), "already pinned") {
					t.Fatalf("Add past an upper-case line: %v", err)
				}
			}
		})
	}
}

func TestTokenizer_SshDivergentLinesAreForeign(t *testing.T) {
	cases := map[string]string{
		"vtab-list":     "junk\v,n1 " + keyA,
		"space-comma":   "junk ,n1 " + keyA,
		"cr-list":       "junk\r,n1 " + keyA,
		"formfeed-list": "junk\f,n1 " + keyA,
		"nel-list":      "junk\u0085,n1 " + keyA,
		"leading-vtab":  "\vn1 " + keyA,
		"vtab-before-k": "n1\v" + keyA,
		"nbsp-sep":      "n1 " + keyA,
		"high-byte":     "né1 " + keyA,
		"del-in-host":   "n\x7f1 " + keyA,
		"control-in-kc": "n1 " + keyA + " note\v",
		"bare-vtab":     "\v",
		"cr-only-eol":   "web " + keyA + "\r@cert-authority * " + keyA,
		"bad-keytype":   "n1 ssh-bogus AAAAB3NzaC1yc2E=",
		"two-fields":    "n1 notatype",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			p, _ := pinFile(t)
			want := "ok " + keyA + "\n" + line + "\n"
			writePin(t, p, want)
			ops := map[string]func() error{
				"Lookup":  func() error { _, err := p.Lookup("n1"); return err },
				"Add":     func() error { return p.Add("n2", keyB) },
				"Replace": func() error { return p.Replace("n1", keyB) },
				"Remove":  func() error { return p.Remove("n1") },
			}
			for op, fn := range ops {
				err := fn()
				if err == nil || !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), p.Path) {
					t.Errorf("%s: %v", op, err)
				} else if strings.Contains(err.Error(), keyA[len("ssh-ed25519 "):]) {
					t.Errorf("%s echoes the key blob: %v", op, err)
				}
			}
			if got := readPin(t, p); got != want {
				t.Errorf("file changed: %q", got)
			}
		})
	}
}

func TestTokenizer_CRLFAcceptedAndPreserved(t *testing.T) {
	p, _ := pinFile(t)
	writePin(t, p, "n1 "+keyA+"\r\n# note\v\r\n\t n3\t"+keyR+"\r\n")
	if got, err := p.Lookup("n1"); err != nil || len(got) != 1 || got[0] != keyA {
		t.Fatalf("Lookup(n1) = %q, %v", got, err)
	}
	if got, err := p.Lookup("n3"); err != nil || len(got) != 1 || got[0] != keyR {
		t.Fatalf("Lookup(n3) = %q, %v", got, err)
	}
	if err := p.Add("n2", keyB); err != nil {
		t.Fatal(err)
	}
	want := "n1 " + keyA + "\r\n# note\v\r\n\t n3\t" + keyR + "\r\nn2 " + keyB + "\n"
	if got := readPin(t, p); got != want {
		t.Fatalf("file = %q", got)
	}
}

func TestTokenizer_Helpers(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{{"", 0}, {" \t ", 0}, {"a b", 2}, {"a\tb  c\r", 3}, {"a\vb", 1}, {"a b", 1}, {"a\u0085b", 1}} {
		if got := len(splitLine(tc.in)); got != tc.want {
			t.Errorf("splitLine(%q) = %d fields, want %d", tc.in, got, tc.want)
		}
	}
	for _, tc := range []struct {
		in   string
		ctl  bool
		high bool
	}{
		{"abc", false, false}, {"a\tb", false, false}, {"a b", false, false}, {"\x1f", true, false},
		{"\x00", true, false}, {"\x0b", true, false}, {"\x7f", true, false}, {"~", false, false},
		{"\x7e", false, false}, {"\x80", false, true}, {"é", false, true}, {"\x7f\x80", true, true},
	} {
		if hasControl(tc.in) != tc.ctl || hasHighByte(tc.in) != tc.high {
			t.Errorf("%q: hasControl=%v hasHighByte=%v", tc.in, hasControl(tc.in), hasHighByte(tc.in))
		}
	}
	if hostFields("# n1 "+keyA) != nil || hostFields("   ") != nil || hostFields("\t#x") != nil {
		t.Error("comment or blank line returned fields")
	}
	if f := hostFields("a b"); len(f) != 2 {
		t.Errorf("hostFields(a b) = %q", f)
	}
}

func TestAlias_LeadingAtRefused(t *testing.T) {
	p, _ := pinFile(t)
	for _, a := range []string{"@revoked", "@cert-authority", "@x"} {
		for op, err := range map[string]error{
			"Add":     p.Add(a, keyA),
			"Replace": p.Replace(a, keyA),
			"Remove":  p.Remove(a),
			"Lookup":  func() error { _, e := p.Lookup(a); return e }(),
		} {
			if err == nil || !strings.Contains(err.Error(), "marker") {
				t.Errorf("%s(%q): %v", op, a, err)
			}
		}
	}
	if a, err := checkPinAlias("a@b"); err != nil || a != "a@b" { // '@' elsewhere is fine
		t.Errorf("a@b = %q, %v", a, err)
	}
	if a, err := checkPinAlias("MiXed-1"); err != nil || a != "mixed-1" {
		t.Errorf("MiXed-1 = %q, %v", a, err)
	}
}

func TestForeign_CommentWithControlIsIgnored(t *testing.T) {
	if bad := foreignLines("# a\vb,*\r\nn1 " + keyA + "\r\n"); len(bad) != 0 {
		t.Fatalf("bad = %q", bad)
	}
	bad := foreignLines("n1 " + keyA + "\n@x " + keyA + "\n\nn? " + keyA + "\n")
	if len(bad) != 2 || !strings.HasPrefix(bad[0], "line 2:") || !strings.HasPrefix(bad[1], "line 4:") {
		t.Fatalf("bad = %q", bad)
	}
}

func TestWrite_RefusesForeignLinesAsLastGuard(t *testing.T) {
	p, _ := pinFile(t)
	err := p.write([]string{"n1 " + keyA, "@revoked n2 " + keyB})
	if err == nil || !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), p.Path) {
		t.Fatalf("write = %v", err)
	}
	if _, serr := os.Stat(p.Path); !os.IsNotExist(serr) {
		t.Error("pin file created despite the refusal")
	}
	if err := p.write([]string{"n1 " + keyA}); err != nil {
		t.Fatal(err)
	}
}

func TestShortLines_AreForeign(t *testing.T) {
	cases := map[string]string{
		"no-final-newline": "zz " + keyA + "\nn1",
		"one-field":        "n1\n",
		"two-fields":       "n1 ssh-ed25519\n",
		"two-fields-crlf":  "n1 ssh-ed25519\r\n",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			p, _ := pinFile(t)
			writePin(t, p, want)
			ops := map[string]func() error{
				"Lookup":  func() error { _, err := p.Lookup("n1"); return err },
				"Add":     func() error { return p.Add("n2", keyB) },
				"Replace": func() error { return p.Replace("n1", keyB) },
				"Remove":  func() error { return p.Remove("n1") },
			}
			for op, fn := range ops {
				if err := fn(); err == nil || !strings.Contains(err.Error(), "fewer than 3 fields") || !strings.Contains(err.Error(), p.Path) {
					t.Errorf("%s: %v", op, err)
				}
			}
			if got := readPin(t, p); got != want {
				t.Errorf("file changed: %q", got)
			}
		})
	}
	if bad := foreignLines("a b c\n"); len(bad) != 1 || !strings.Contains(bad[0], "key type") {
		t.Errorf("three fields with a bad type = %q", bad)
	}
	if bad := foreignLines("a ssh-ed25519 K\n"); len(bad) != 0 {
		t.Errorf("three valid fields flagged: %q", bad)
	}
}
