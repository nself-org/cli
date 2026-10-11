package compose_test

// composeenv_dotenv_test.go — D-0333 round trip: every value nself build
// writes to .nself/compose.env must come back byte for byte through docker
// compose's own dotenv parser.
//
// parseComposeDotenv is a line-for-line port of compose-go/v2 dotenv
// (parser.go at v2.16.1, the parser `docker compose --env-file` uses), with
// template.Substitute reduced to the forms it can meet here: $$, $NAME,
// ${NAME} and ${NAME:-default}. The PR evidence checks the same corpus
// against a real `docker compose config` on compose 2.20.3 through 5.6.0.

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
)

var (
	dotenvEscapeSeq = regexp.MustCompile(`(\\(?:[abcfnrtv$"\\]|0\d{0,3}))`)
	dotenvExport    = regexp.MustCompile(`^export\s+`)
	dotenvVarRef    = regexp.MustCompile(`\$(?:(\$)|([_a-zA-Z][_a-zA-Z0-9]*)|\{([_a-zA-Z][_a-zA-Z0-9]*)(?::-([^}]*))?\})`)
)

// parseComposeDotenv parses src the way compose-go/v2 dotenv.Parse does.
func parseComposeDotenv(src string) (map[string]string, error) {
	p := &dotenvParser{line: 1}
	out := map[string]string{}
	cutset := src
	for {
		cutset = p.statementStart(cutset)
		if cutset == "" {
			return out, nil
		}
		key, left, inherited, err := p.keyName(cutset)
		if err != nil {
			return nil, err
		}
		if strings.Contains(key, " ") {
			return nil, fmt.Errorf("line %d: key cannot contain a space", p.line)
		}
		if inherited {
			cutset = left
			continue
		}
		value, rest, err := p.value(left, out)
		if err != nil {
			return nil, err
		}
		out[key] = value
		cutset = rest
	}
}

type dotenvParser struct{ line int }

func (p *dotenvParser) statementStart(src string) string {
	pos := strings.IndexFunc(src, func(r rune) bool {
		if r == '\n' {
			p.line++
		}
		return !unicode.IsSpace(r)
	})
	if pos == -1 {
		return ""
	}
	src = src[pos:]
	if src[0] != '#' {
		return src
	}
	pos = strings.IndexByte(src, '\n')
	if pos == -1 {
		return ""
	}
	return p.statementStart(src[pos:])
}

func (p *dotenvParser) keyName(src string) (string, string, bool, error) {
	var key string
	var inherited bool
	if dotenvExport.MatchString(src) {
		src = strings.TrimLeftFunc(strings.TrimPrefix(src, "export"), dotenvIsSpace)
	}
	offset := 0
loop:
	for i, r := range src {
		if dotenvIsSpace(r) {
			continue
		}
		switch r {
		case '=', ':', '\n':
			key = src[0:i]
			offset = i + 1
			inherited = r == '\n'
			break loop
		case '_', '.', '-', '[', ']':
		default:
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				continue
			}
			return "", "", inherited, fmt.Errorf("line %d: unexpected character %q in variable name", p.line, string(r))
		}
	}
	if src == "" {
		return "", "", inherited, errors.New("zero length string")
	}
	if inherited && strings.IndexByte(key, ' ') == -1 {
		p.line++
	}
	key = strings.TrimRightFunc(key, unicode.IsSpace)
	return key, strings.TrimLeftFunc(src[offset:], dotenvIsSpace), inherited, nil
}

func (p *dotenvParser) value(src string, env map[string]string) (string, string, error) {
	if src == "" || (src[0] != '\'' && src[0] != '"') {
		value, rest, _ := strings.Cut(src, "\n")
		p.line++
		value, _, _ = strings.Cut(value, " #")
		value = strings.TrimRightFunc(value, unicode.IsSpace)
		return dotenvSubstitute(value, env), rest, nil
	}
	quote := src[0]
	escaped := false
	var chars []byte
	for i := 1; i < len(src); i++ {
		c := src[i]
		if c == '\n' {
			p.line++
		}
		if c != quote {
			if !escaped && c == '\\' {
				escaped = true
				continue
			}
			if escaped {
				escaped = false
				chars = append(chars, '\\')
			}
			chars = append(chars, c)
			continue
		}
		if escaped {
			escaped = false
			chars = append(chars, c)
			continue
		}
		value := string(chars)
		if quote == '"' {
			value = dotenvSubstitute(dotenvExpandEscapes(value), env)
		}
		return value, src[i+1:], nil
	}
	return "", "", fmt.Errorf("line %d: unterminated quoted value", p.line)
}

func dotenvExpandEscapes(s string) string {
	return dotenvEscapeSeq.ReplaceAllStringFunc(s, func(m string) string {
		if m == `\$` {
			return "$$"
		}
		if strings.HasPrefix(m, `\0`) {
			m = strings.Replace(m, `\0`, `\`, 1)
		}
		v, _, _, err := strconv.UnquoteChar(m, '"')
		if err != nil {
			return m
		}
		return string(v)
	})
}

func dotenvSubstitute(s string, env map[string]string) string {
	return dotenvVarRef.ReplaceAllStringFunc(s, func(m string) string {
		g := dotenvVarRef.FindStringSubmatch(m)
		switch {
		case g[1] != "":
			return "$"
		case g[2] != "":
			return env[g[2]]
		case env[g[3]] != "":
			return env[g[3]]
		default:
			return g[4]
		}
	})
}

func dotenvIsSpace(r rune) bool {
	switch r {
	case '\t', '\v', '\f', '\r', ' ', 0x85, 0xA0:
		return true
	}
	return false
}

// dotenvCorpus holds the value shapes that broke or could break compose.env.
// All are synthetic; none is a real credential.
var dotenvCorpus = []string{
	"plain-value",
	"$2a$10$abcdefghijklmnopqrstuuW0bS3cr3tHashOnlyForTests1234", // bcrypt shape
	"ab #cd",
	"ab#cd",
	" leading space",
	"trailing space ",
	"tab\tinside",
	"line1\nline2",
	"it's",
	`say "hi"`,
	`\`,
	`ends\`,
	`ends\\`,
	`ends\\\`,
	`odd\'quote`,
	`even\\'quote`,
	`back\slash`,
	`\n is not a newline`,
	`\$notavar`,
	"${HOME}",
	"$HOME",
	"$$double",
	"'wrapped'",
	`"wrapped"`,
	"`tick`",
	`{"type":"HS256","key":"k\"q"}`,
	"#leading-hash",
	"üñíçødé key",
	"=equals=",
	"a\\\"b'c$d #e",
}

// TestDotenvParserModelsTheBug is the positive control: unquoted values are
// altered by the parser exactly as D-0333 describes, so the round-trip tests
// below can fail.
func TestDotenvParserModelsTheBug(t *testing.T) {
	got, err := parseComposeDotenv("A=$2a$10$abc\nB=ab #cd\nC=  x  \n")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "$2a$10", "B": "ab", "C": "x"} // $abc expands to ""
	for k, v := range want {
		if got[k] != v {
			t.Errorf("unquoted %s = %q, want %q (the parser no longer models compose)", k, got[k], v)
		}
	}
}

func TestQuoteComposeEnvValueRoundTrip(t *testing.T) {
	values := append([]string(nil), dotenvCorpus...)
	rng := rand.New(rand.NewSource(333))
	const alphabet = "ab \t#$'\"\\`{}=:\n"
	for i := 0; i < 2000; i++ {
		b := make([]byte, 1+rng.Intn(12))
		for j := range b {
			b[j] = alphabet[rng.Intn(len(alphabet))]
		}
		values = append(values, string(b))
	}
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue // SecretEnvMap never carries a blank value
		}
		q := config.QuoteComposeEnvValue(v)
		if strings.HasPrefix(q, "'") && strings.ContainsAny(q[1:len(q)-1], `'\`) {
			// compose before 2.24 reads \' and a backslash before the
			// closing quote differently; such values must be double-quoted.
			t.Errorf("%q: single-quoted form %q is not portable", v, q)
		}
		line := "K=" + q + "\n"
		got, err := parseComposeDotenv(line)
		if err != nil {
			t.Errorf("%q: parse %q: %v", v, line, err)
			continue
		}
		if got["K"] != v {
			t.Errorf("round trip of %q via %q = %q", v, line, got["K"])
		}
	}
}

func FuzzQuoteComposeEnvValue(f *testing.F) {
	for _, v := range dotenvCorpus {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v string) {
		if strings.TrimSpace(v) == "" || strings.ContainsRune(v, 0) {
			return
		}
		got, err := parseComposeDotenv("K=" + config.QuoteComposeEnvValue(v) + "\n")
		if err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		if got["K"] != v {
			t.Fatalf("round trip of %q = %q", v, got["K"])
		}
	})
}

// TestComposeEnvFileRoundTrip writes .nself/compose.env through the nself
// build path with every secret set to an awkward synthetic value and parses
// the whole file back the way docker compose does.
func TestComposeEnvFileRoundTrip(t *testing.T) {
	i := 0
	awkward := func(base string) string {
		v := base + dotenvCorpus[i%len(dotenvCorpus)]
		i++
		return v
	}
	cfg := optionalSecretsConfig("typesense", awkward)
	cfg.Postgres.Password = "pg$2a$10 #x'y\\"
	cfg.Redis.Password = `redis"\'$HOME #z\`
	secrets := build.SecretEnvMap(cfg)

	dir := t.TempDir()
	if err := build.WriteComposeEnv(dir, cfg, secrets, map[string]string{"PLUGIN_X": "v w #x"}); err != nil {
		t.Fatalf("WriteComposeEnv: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".nself", "compose.env"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseComposeDotenv(string(raw))
	if err != nil {
		t.Fatalf("compose would reject compose.env: %v", err)
	}
	want := map[string]string{"PLUGIN_X": "v w #x", "DATABASE_URL": cfg.DatabaseURL()}
	for k, v := range secrets {
		want[k] = v
	}
	want["POSTGRES_PASSWORD"+config.URLPasswordEnvSuffix] = config.URLPassword(cfg.Postgres.Password)
	want["REDIS_PASSWORD"+config.URLPasswordEnvSuffix] = config.URLPassword(cfg.Redis.Password)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: compose reads %q, want %q", k, got[k], v)
		}
	}
}
