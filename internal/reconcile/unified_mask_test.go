package reconcile

import "testing"

// TestApplySecretValuesOverlap: overlapping values hide the whole run, not
// just the leftmost match ("link" and "ink-supersecret123" in one word).
func TestApplySecretValuesOverlap(t *testing.T) {
	got := applySecretValues([]string{"link-supersecret123\n", "x link y ink-supersecret123 z\n", "[REDACTED]link\n", "none\n"},
		[]string{"link", "ink-supersecret123"})
	want := []string{"[REDACTED]\n", "x [REDACTED] y [REDACTED] z\n", "[REDACTED][REDACTED]\n", "none\n"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
	runShapes(t, []shapeCase{{
		name:    "through Unified",
		before:  "A_SECRET: link\nB_SECRET: ink-supersecret123\nnote: link-supersecret123 one\n",
		after:   "A_SECRET: link\nB_SECRET: ink-supersecret123\nnote: link-supersecret123 two\n",
		secrets: []string{"supersecret123"},
		want:    []string{"note: [REDACTED] two\n"},
	}})
}

// TestUnifiedMultipleDirectives: several directives on one line are handled
// one by one; an escaped ; or quote inside a secret argument does not end it.
func TestUnifiedMultipleDirectives(t *testing.T) {
	runShapes(t, []shapeCase{
		{name: "two directives", path: "nginx/sites/a.conf",
			before:  "  proxy_set_header X-Plugin-Secret fakeheader123; proxy_pass http://x:8080;\n  add_header X-Id fakeheader123;\n",
			after:   "  proxy_set_header X-Plugin-Secret fakeheader124; proxy_pass http://x:8080;\n  add_header X-Id fakeheader123;\n",
			secrets: []string{"fakeheader123", "fakeheader124"},
			want:    []string{"X-Plugin-Secret [REDACTED]; proxy_pass http://x:8080;\n", "add_header X-Id [REDACTED];"}},
		{name: "secret in the second directive", path: "nginx/sites/a.conf",
			before:  "  proxy_pass http://x:8080; set $api_token fakeTok123;\n",
			after:   "  proxy_pass http://x:9090; set $api_token fakeTok123;\n",
			secrets: []string{"fakeTok123"},
			want:    []string{"proxy_pass http://x:8080; set $api_token [REDACTED];\n"}},
		{name: "escaped semicolon", path: "nginx/sites/a.conf",
			before:  "  set $api_token abc\\;fakeescape1; # c\n  add_header X abc\\;fakeescape1;\n",
			after:   "  set $api_token abc\\;fakeescape1; # d\n  add_header X abc\\;fakeescape1;\n",
			secrets: []string{"fakeescape1"},
			want:    []string{"set $api_token [REDACTED]; # c\n"}},
		{name: "escaped quote", path: "nginx/sites/a.conf",
			before:  "  set $api_token \"ab\\\"fakequote1;x\"; listen 80;\n",
			after:   "  set $api_token \"ab\\\"fakequote1;x\"; listen 81;\n",
			secrets: []string{"fakequote1"},
			want:    []string{"set $api_token [REDACTED]; listen 80;\n"}},
	})
}

// TestUnifiedCommentInsideQuotes: a # inside quotes is text, not a comment, so
// the whole quoted value is collected and a repeat elsewhere is hidden.
func TestUnifiedCommentInsideQuotes(t *testing.T) {
	var tests []shapeCase
	for _, c := range []struct{ name, val string }{
		{"double quoted", `"start # continuationsecret"`},
		{"single quoted", `'start # continuationsecret'`},
	} {
		tests = append(tests, shapeCase{
			name: c.name, secrets: []string{"continuationsecret"},
			before: "command:\n  - --requirepass\n  - " + c.val + " # real comment\n  - echo old\n",
			after:  "command:\n  - --requirepass\n  - " + c.val + " # real comment\n  - echo start # continuationsecret\n",
			want:   []string{"  - echo [REDACTED]\n"},
		})
	}
	runShapes(t, tests)
}

// TestStripCommentQuotes pins the helper: quotes protect a #, an apostrophe in
// a word does not open a quote.
func TestStripCommentQuotes(t *testing.T) {
	for in, want := range map[string]string{
		`abc # note`:           "abc",
		"abc\t# note":          "abc",
		`"a # b" # note`:       `"a # b"`,
		`'a # b'`:              `'a # b'`,
		`it's fine # note`:     "it's fine",
		`a#b # note`:           "a#b",
		`"esc \" # still" # n`: `"esc \" # still"`,
	} {
		if got := stripComment(in); got != want {
			t.Errorf("stripComment(%q) = %q, want %q", in, got, want)
		}
	}
}
