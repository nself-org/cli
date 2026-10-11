package reconcile

import (
	"strings"
	"testing"
)

// shapeCase is one artifact shape for the diff redaction tests. before and
// after are the two sides (before may be empty for a new file).
type shapeCase struct {
	name, path, before, after string
	secrets                   []string // must be in the raw input, never in the diff
	want                      []string // must be in the diff as written
}

// runShapes checks each case: positive control on the raw input, no secret in
// the diff, every wanted string present.
func runShapes(t *testing.T, tests []shapeCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.path == "" {
				tc.path = "docker-compose.yml"
			}
			for _, s := range tc.secrets {
				if !strings.Contains(tc.before+tc.after, s) {
					t.Fatalf("positive control: input lacks %q", s)
				}
			}
			got := Unified(tc.path, []byte(tc.before), []byte(tc.after))
			if got == "" {
				t.Fatal("empty diff")
			}
			for _, s := range tc.secrets {
				if strings.Contains(got, s) {
					t.Errorf("diff leaks %q:\n%s", s, got)
				}
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("diff lacks %q:\n%s", w, got)
				}
			}
			if strings.Contains(got, "]]") || strings.Contains(got, "\x00") {
				t.Errorf("diff holds a doubled bracket or NUL:\n%s", got)
			}
		})
	}
}

// TestUnifiedFlagValuesWithSpecials covers an unquoted flag value that holds
// spaces, quotes and $ (M2), plain and inside a YAML quoted command line.
func TestUnifiedFlagValuesWithSpecials(t *testing.T) {
	mk := func(pre, val, post string) string {
		return "    command: " + pre + "redis-server --appendonly yes --protected-mode yes --requirepass " + val + " --maxclients 100" + post + "\n"
	}
	var tests []shapeCase
	for _, v := range []string{"pa ss Hunter2Leak", "Hunter2'Leak", `Hunter2"Leak`, "Hunter2Leak$x", `Hunter2\Leak`} {
		tests = append(tests, shapeCase{
			name: "plain " + v, secrets: []string{"Hunter2"}, before: mk("", v, ""), after: mk("", v+"2", ""),
			want: []string{"--requirepass [REDACTED] --maxclients 100\n"},
		})
	}
	tests = append(tests,
		shapeCase{name: "yaml single-quoted", secrets: []string{"Hunter2"}, before: mk("'", "Hunter2''Leak", "'"), after: mk("'", "Hunter2''Leak2", "'"),
			want: []string{"--requirepass [REDACTED] --maxclients 100'\n"}},
		shapeCase{name: "yaml double-quoted", secrets: []string{"Hunter2"}, before: mk(`"`, `Hunter2\"Leak`, `"`), after: mk(`"`, `Hunter2\"Leak2`, `"`),
			want: []string{"--requirepass [REDACTED] --maxclients 100\"\n"}},
		shapeCase{name: "equals form", secrets: []string{"Hunter2"}, before: "    command: --data-dir /data --api-key=Hunter2 Leak --enable-cors\n", after: "    command: --data-dir /data --api-key=Hunter2 Leak2 --enable-cors\n",
			want: []string{"--api-key=[REDACTED] --enable-cors\n"}},
	)
	runShapes(t, tests)
}

// TestUnifiedRepeatedValues covers the same value in another line of the file
// (M3), including a value hidden only on the other side of the diff.
func TestUnifiedRepeatedValues(t *testing.T) {
	runShapes(t, []shapeCase{
		{
			name:    "healthcheck list items",
			before:  "    healthcheck:\n      test:\n        - CMD\n        - redis-cli\n        - -a\n        - fake-pw-0000\n        - ping\n    command: redis-server --requirepass fake-pw-0000 --maxclients 50\n",
			after:   "    healthcheck:\n      test:\n        - CMD\n        - redis-cli\n        - -a\n        - fake-pw-0000\n        - ping\n    command: redis-server --requirepass fake-pw-0000 --maxclients 60\n",
			secrets: []string{"fake-pw-0000"},
			want:    []string{"         - -a\n", "         - [REDACTED]\n"},
		},
		{
			name:    "flow list",
			before:  "    command: redis-server --requirepass fake-pw-0000\n    test: [CMD, redis-cli, -a, fake-pw-0000, ping]\n",
			after:   "    command: redis-server --requirepass fake-pw-0000 --x\n    test: [CMD, redis-cli, -a, fake-pw-0000, ping]\n",
			secrets: []string{"fake-pw-0000"},
			want:    []string{"test: [CMD, redis-cli, -a, [REDACTED], ping]"},
		},
		{
			name:    "value only secret on the other side",
			before:  "    command: redis-server --requirepass old-pw-0000\n    note: keep old-pw-0000 here\n",
			after:   "    command: redis-server --requirepass new-pw-1111\n    note: keep old-pw-0000 here\n",
			secrets: []string{"old-pw-0000", "new-pw-1111"},
			want:    []string{"note: keep [REDACTED] here"},
		},
		{
			name:    "short value is not spread",
			before:  "    environment:\n      API_TOKEN: abc\n      NAME: abc-service\n",
			after:   "    environment:\n      API_TOKEN: abd\n      NAME: abc-service\n",
			secrets: []string{"API_TOKEN: abc\n"},
			want:    []string{"NAME: abc-service"},
		},
	})
}

// TestUnifiedSecretScopes covers a secret key whose value is empty, a comment,
// an anchor or tag, or continues on deeper lines (S1).
func TestUnifiedSecretScopes(t *testing.T) {
	runShapes(t, []shapeCase{
		{name: "empty value, deeper line", before: "x:\n  DB_PASSWORD:\n    fake-0000-a\n  NAME: keep-me\n", after: "x:\n  DB_PASSWORD:\n    fake-0000-b\n  NAME: keep-me\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"NAME: keep-me"}},
		{name: "comment only", before: "x:\n  API_TOKEN: # note\n    - fake-0000-a\n  NAME: keep-me\n", after: "x:\n  API_TOKEN: # note\n    - fake-0000-b\n  NAME: keep-me\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"NAME: keep-me"}},
		{name: "anchor and block", before: "x:\n  PRIVATE_KEY: &k |\n    fake-0000-a\n  NAME: keep-me\n", after: "x:\n  PRIVATE_KEY: &k |\n    fake-0000-b\n  NAME: keep-me\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"PRIVATE_KEY: [REDACTED]", "NAME: keep-me"}},
		{name: "tag and block", before: "x:\n  PRIVATE_KEY: !!str |\n    fake-0000-a\n  NAME: keep-me\n", after: "x:\n  PRIVATE_KEY: !!str |\n    fake-0000-b\n  NAME: keep-me\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"PRIVATE_KEY: [REDACTED]", "NAME: keep-me"}},
		{name: "plain value continues", before: "x:\n  API_KEY: part-0000\n    cont-0000-a\n  NAME: keep-me\n", after: "x:\n  API_KEY: part-0000\n    cont-0000-b\n  NAME: keep-me\n",
			secrets: []string{"cont-0000-a", "cont-0000-b"}, want: []string{"NAME: keep-me"}},
		{name: "list item key", before: "x:\n  - API_KEY: fake-0000-a\n    NAME: keep-me\n", after: "x:\n  - API_KEY: fake-0000-b\n    NAME: keep-me\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"NAME: keep-me"}},
	})
}

// TestUnifiedInlinePairs covers ” escapes, flow maps, headers, JSON scalars
// and `password = "v"` (S2, S3), and that URLs and plain words stay.
func TestUnifiedInlinePairs(t *testing.T) {
	runShapes(t, []shapeCase{
		{name: "single-quote escape in list item", before: "  - 'POSTGRES_PASSWORD=ab''hunter2-0000'\n  - 'N=1'\n", after: "  - 'POSTGRES_PASSWORD=ab''hunter2-1111'\n  - 'N=1'\n",
			secrets: []string{"hunter2"}, want: []string{"'N=1'"}},
		{name: "single-quote escape across lines", before: "KEY_TOKEN: 'it''s\n  hunter2-0000'\nNAME: keep-me\n", after: "KEY_TOKEN: 'it''s\n  hunter2-1111'\nNAME: keep-me\n",
			secrets: []string{"hunter2"}, want: []string{"NAME: keep-me"}},
		{name: "flow map", before: "    environment: {POSTGRES_PASSWORD: fake-0000-a, B: keep-b}\n", after: "    environment: {POSTGRES_PASSWORD: fake-0000-b, B: keep-b}\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"{POSTGRES_PASSWORD: [REDACTED], B: keep-b}"}},
		{name: "header in quoted string", before: "    test: \"curl -H 'x-hasura-admin-secret: fake-0000-a' http://h:8080\"\n", after: "    test: \"curl -H 'x-hasura-admin-secret: fake-0000-b' http://h:8080\"\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"http://h:8080"}},
		{name: "header as list item", before: "  - \"x-hasura-admin-secret: fake-0000-a\"\n  - keep\n", after: "  - \"x-hasura-admin-secret: fake-0000-b\"\n  - keep\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"  - keep\n", "- \"x-hasura-admin-secret: [REDACTED]\"\n"}},
		{name: "json number", path: "s.json", before: "{\"a\": 1, \"API_KEY\":12345678, \"b\": 2}\n", after: "{\"a\": 1, \"API_KEY\":87654321, \"b\": 2}\n",
			secrets: []string{"12345678", "87654321"}, want: []string{"\"b\": 2}"}},
		{name: "toml spacing", path: "s.toml", before: "name = \"keep\"\npassword = \"fake-0000-a\"\n", after: "name = \"keep\"\npassword = \"fake-0000-b\"\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"password = \"[REDACTED]\"", "name = \"keep\""}},
		{name: "urls stay", before: "    url: http://host:8080/x\n    t: fake-0000-a\n    API_KEY: fake-0000-a\n", after: "    url: http://host:8080/y\n    t: fake-0000-a\n    API_KEY: fake-0000-a\n",
			secrets: []string{"fake-0000-a"}, want: []string{"+    url: http://host:8080/y\n"}},
	})
}

// TestUnifiedFlagValueOnNextLine covers a bare flag whose value is the next
// list item, with blank and comment lines between (S4), and the doubled
// bracket of the typesense command (S5).
func TestUnifiedFlagValueOnNextLine(t *testing.T) {
	runShapes(t, []shapeCase{
		{name: "comment between", before: "    command:\n      - redis-server\n      - --requirepass\n      # note\n\n      - fake-pw-0000\n      - --a\n", after: "    command:\n      - redis-server\n      - --requirepass\n      # note\n\n      - fake-pw-1111\n      - --a\n",
			secrets: []string{"fake-pw-0000", "fake-pw-1111"}, want: []string{"      - --a\n", "# note"}},
		{name: "typesense command", before: "    command: --data-dir /data --api-key=fake-0000-a --enable-cors\n", after: "    command: --data-dir /data --api-key=fake-0000-b --enable-cors\n",
			secrets: []string{"fake-0000-a", "fake-0000-b"}, want: []string{"--api-key=[REDACTED] --enable-cors\n"}},
	})
}

// TestRedactSecretLinesNeverPanics runs fixed hostile lines (including the
// slice-bounds case: a secret pair after the closing quote of its scalar) and
// every line of up to four tokens from a hostile alphabet through the
// redaction, checking the line count.
func TestRedactSecretLinesNeverPanics(t *testing.T) {
	check := func(prefix string) {
		in := []string{prefix + "\n", prefix}
		if out := redactSecretLines(in); len(out) != len(in) {
			t.Fatalf("line count changed for %q", prefix)
		}
	}
	for _, l := range []string{`command: "foo" API_KEY=abc`, `x: 'a' API_KEY: v`, `- "a" --pass b`, `a: "b" # c: API_KEY=d`} {
		check(l)
	}
	toks := []string{"API_KEY", "x", "--pass", "-", "=", ":", " ", `"`, "'", ",", "{", "\\"}
	var gen func(prefix string, n int)
	gen = func(prefix string, n int) {
		if n == 0 {
			check(prefix)
			return
		}
		for _, tok := range toks {
			gen(prefix+tok, n-1)
		}
	}
	for n := 1; n <= 4; n++ {
		gen("", n)
	}
}

// TestRedactSecretLinesBlankStaysBlank: a blank line inside a secret scope
// prints blank, and the line count is unchanged.
func TestRedactSecretLinesBlankStaysBlank(t *testing.T) {
	in := []string{"PRIVATE_KEY: |\n", "  line-one\n", "\n", "  line-two\n", "NAME: keep\n"}
	out := redactSecretLines(in)
	if len(out) != len(in) || out[2] != "\n" || out[1] != "  [REDACTED]\n" || out[4] != "NAME: keep\n" {
		t.Fatalf("got %q", out)
	}
}

// TestUnifiedSwappedValuesAcrossSides: each side holds the other side's secret
// in a plain line. Collection must span both sides (through Unified).
func TestUnifiedSwappedValuesAcrossSides(t *testing.T) {
	runShapes(t, []shapeCase{{
		name: "swapped", secrets: []string{"alphapass", "betapass"},
		before: "PASSWORD: alphapass\ncommand: echo betapass\n",
		after:  "PASSWORD: betapass\ncommand: echo alphapass\n",
		want:   []string{"command: echo [REDACTED]\n"},
	}})
}

// TestUnifiedCollectsScalarText: the collected value is the scalar text, not
// the YAML around it (block content, anchor, tag, trailing comment, plain and
// quoted continuation lines), so a repeat elsewhere is hidden.
func TestUnifiedCollectsScalarText(t *testing.T) {
	cases := []struct{ name, lines, pw string }{
		{"block scalar", "PASSWORD: |\n  blocksecret\n", "blocksecret"},
		{"anchor", "PASSWORD: &pw anchorsecret\n", "anchorsecret"},
		{"tag", "PASSWORD: !!str tagsecret\n", "tagsecret"},
		{"comment", "PASSWORD: commentsecret # note\n", "commentsecret"},
		{"plain continuation", "PASSWORD: firstpart\n  contsecret\n", "contsecret"},
		{"quoted continuation", "PASSWORD: \"quotedfirst\n  quotedsecret\"\n", "quotedsecret"},
		{"quoted anchor", "PASSWORD: &pw \"qanchorsecret\" # note\n", "qanchorsecret"},
	}
	var tests []shapeCase
	for _, c := range cases {
		tests = append(tests, shapeCase{
			name: c.name, secrets: []string{c.pw},
			before: c.lines + "command: echo old\n", after: c.lines + "command: echo " + c.pw + "\n",
			want: []string{"command: echo [REDACTED]\n"},
		})
	}
	runShapes(t, tests)
}

// TestUnifiedDirectives: nginx-style directives. The first token (the
// directive name) is never tested; a later token that names a secret hides the
// arguments after it, and they are collected. Other directives print as is.
func TestUnifiedDirectives(t *testing.T) {
	runShapes(t, []shapeCase{
		{name: "header", path: "nginx/sites/a.conf",
			before:  "  listen 443 ssl;\n  proxy_set_header X-Plugin-Secret fake-header-secret;\n  proxy_pass http://x:8080;\n",
			after:   "  listen 443 ssl;\n  proxy_set_header X-Plugin-Secret fake-header-secret2;\n  proxy_pass http://x:8080;\n",
			secrets: []string{"fake-header-secret"},
			want:    []string{"  proxy_set_header X-Plugin-Secret [REDACTED];\n", "  listen 443 ssl;\n", "  proxy_pass http://x:8080;\n"}},
		{name: "set variable with comment", path: "nginx/sites/a.conf",
			before:  "  set $api_token fakeTok123; # rotate\n  add_header X-Id fakeTok123;\n",
			after:   "  set $api_token fakeTok124; # rotate\n  add_header X-Id fakeTok123;\n",
			secrets: []string{"fakeTok123", "fakeTok124"},
			want:    []string{"  set $api_token [REDACTED]; # rotate\n", "add_header X-Id [REDACTED];"}},
		{name: "directive name is not tested", path: "nginx/sites/a.conf",
			before: "  ssl_certificate_key /etc/x.key;\n  proxy_pass http://x:8080;\n  listen 443 ssl;\n",
			after:  "  ssl_certificate_key /etc/y.key;\n  proxy_pass http://x:9090;\n  listen 443 ssl;\n",
			want:   []string{"-  ssl_certificate_key /etc/x.key;\n", "+  proxy_pass http://x:9090;\n", "  listen 443 ssl;\n"}},
	})
}
