package build

// Purpose: D-0635 regression tests for TemplateSecrets: percent-encoded URL
//          credentials, YAML-escaped literals, and the invariant that a
//          rewrite keeps the YAML loadable and every resolved value equal to
//          the original.

import (
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/config"
)

// TestTemplateSecrets_URLEscapedPasswordUsesEncodedTwin: a password that
// needs percent-encoding is never substituted raw into a URL (compose would
// inject the unescaped value); its encoded literal becomes a reference to the
// _URLENC twin, so no "$" in it is left for compose to interpolate (D-0635).
func TestTemplateSecrets_URLEscapedPasswordUsesEncodedTwin(t *testing.T) {
	const twin = ":${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@"
	cases := []struct{ name, pw, want string }{
		{"space-percent", "has space%pw", twin},
		{"dollar-hash", `fakepw$abc#def ghi"jk`, twin},
		{"dollar-brace", "fakepw${x}notreal", twin},
		{"reserved", "fake@pw:/?notreal", twin},
		{"short-dollar", "a$b", twin},
		// URL-safe passwords keep the Phase B bytes they always had.
		{"url-safe", "fakepwnotreal0123", ":${POSTGRES_PASSWORD}@"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			yaml := "      DATABASE_URL: postgresql://postgres:" + config.URLPassword(c.pw) + "@postgres:5432/db\n"
			out := string(TemplateSecrets([]byte(yaml), map[string]string{"POSTGRES_PASSWORD": c.pw}))
			if !strings.Contains(out, c.want) {
				t.Errorf("want %s in:\n%s", c.want, out)
			}
			if strings.Contains(out, config.URLPassword(c.pw)+"@") {
				t.Errorf("encoded password left literal:\n%s", out)
			}
		})
	}
}

// TestTemplateSecrets_EscapedLiteralsRewritten: a secret the YAML encoder
// escaped (quotes, backslashes) is still found, on alias lines (Phase A2)
// and anywhere else (Phase C), and the rewritten line stays valid YAML.
func TestTemplateSecrets_EscapedLiteralsRewritten(t *testing.T) {
	pg := `fake"pw\$abc`
	minio := `minio"pw\$abc`
	in := "services:\n  auth:\n    environment:\n      AUTH_DB_PASSWORD: \"fake\\\"pw\\\\$abc\"\n" +
		"  api:\n    environment:\n      S3_SECRET_KEY: \"minio\\\"pw\\\\$abc\"\n      OTHER: \"x minio\\\"pw\\\\$abc y\"\n"
	out := string(TemplateSecrets([]byte(in), map[string]string{"POSTGRES_PASSWORD": pg, "MINIO_ROOT_PASSWORD": minio}))
	for _, want := range []string{
		"AUTH_DB_PASSWORD: ${POSTGRES_PASSWORD}",
		"S3_SECRET_KEY: ${MINIO_ROOT_PASSWORD}",
		"OTHER: x ${MINIO_ROOT_PASSWORD} y",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "$abc") {
		t.Errorf("escaped literal left in:\n%s", out)
	}
}

// envDoc is a minimal compose document: one service with an env map.
type envDoc struct {
	Services map[string]struct {
		Environment map[string]string `yaml:"environment"`
	} `yaml:"services"`
}

// marshalEnv renders env the way the generator does (yaml.v3, two-space
// indent) under services.app.environment.
func marshalEnv(t *testing.T, env map[string]string) []byte {
	t.Helper()
	doc := map[string]any{"services": map[string]any{"app": map[string]any{"environment": env}}}
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		t.Fatal(err)
	}
	return []byte(sb.String())
}

// testRefPattern matches the references TemplateSecrets writes.
var testRefPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)(?::-(?:\$\{([A-Z0-9_]+)\})?)?\}`)

// resolveSecrets substitutes TemplateSecrets references from secrets (plus
// the _URLENC twins), the way compose resolves them from compose.env.
func resolveSecrets(v string, secrets map[string]string) string {
	return testRefPattern.ReplaceAllStringFunc(v, func(ref string) string {
		m := testRefPattern.FindStringSubmatch(ref)
		if base, ok := strings.CutSuffix(m[1], config.URLPasswordEnvSuffix); ok {
			if pw, ok := secrets[base]; ok {
				return config.URLPassword(pw)
			}
		}
		if pw, ok := secrets[m[1]]; ok {
			return pw
		}
		return ref
	})
}

// templatedEnv templates doc and returns the env map it decodes to, failing
// when the result is not loadable YAML.
func templatedEnv(t *testing.T, doc []byte, secrets map[string]string) (map[string]string, string) {
	t.Helper()
	out := TemplateSecrets(doc, secrets)
	var got envDoc
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("templated YAML does not load: %v\n%s", err, out)
	}
	return got.Services["app"].Environment, string(out)
}

// TestTemplateSecrets_QuotedEdgesStayLoadable: a secret whose quote sits at
// the edge of a YAML-quoted scalar (” escape) or that ends in a backslash
// inside a double-quoted scalar must not be byte-replaced through the escape
// (Opus/lens finding on cli#697); the line is rewritten via its decoded value.
func TestTemplateSecrets_QuotedEdgesStayLoadable(t *testing.T) {
	cases := []struct{ name, secret, value string }{
		{"single-quote-edges", "'fakepwnotreal'", "'fakepwnotreal'"},
		{"single-quote-end", "@fake$abcpw'", "@fake$abcpw'"},
		{"backslash-end", `fakepwnotreal\`, `a fakepwnotreal\ "b"`},
		{"tab", "fake\tpwnotreal", "x fake\tpwnotreal y"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			secrets := map[string]string{"MINIO_ROOT_PASSWORD": c.secret}
			env, out := templatedEnv(t, marshalEnv(t, map[string]string{"S3_SECRET_KEY": c.value}), secrets)
			got := env["S3_SECRET_KEY"]
			if strings.Contains(got, c.secret) {
				t.Errorf("secret left literal:\n%s", out)
			}
			if r := resolveSecrets(got, secrets); r != c.value {
				t.Errorf("resolves to %q, want %q", r, c.value)
			}
		})
	}
}

// TestTemplateSecrets_RefsAndBlockScalarsUntouched: a secret that spells
// part of an earlier reference does not rewrite inside it, and text inside a
// block scalar is never re-encoded.
func TestTemplateSecrets_RefsAndBlockScalarsUntouched(t *testing.T) {
	in := "services:\n  app:\n    environment:\n      POSTGRES_PASSWORD: x\n" +
		"    command: |\n      note: \"fake\\\"pw\\\\$abc#def\"\n"
	secrets := map[string]string{
		"POSTGRES_PASSWORD":           "pgvalue-notreal",
		"HASURA_GRAPHQL_ADMIN_SECRET": "PASSWORD}",
		"MINIO_ROOT_PASSWORD":         `fake"pw\$abc#def`,
	}
	out := string(TemplateSecrets([]byte(in), secrets))
	if !strings.Contains(out, "POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}\n") {
		t.Errorf("reference broken by a secret spelling part of it:\n%s", out)
	}
	if !strings.Contains(out, `note: "fake\"pw\\$abc#def"`) {
		t.Errorf("block scalar text was rewritten:\n%s", out)
	}
	for _, header := range []string{"|", "|-", ">+", "|2-", "| # c", "!!str |", "&a |"} {
		doc := "services:\n  app:\n    command: " + header + "\n      note: \"fake\\\"pw\\\\$abc#def\"\n"
		if got := string(TemplateSecrets([]byte(doc), secrets)); got != doc {
			t.Errorf("block scalar %q text was rewritten:\n%s", header, got)
		}
	}
	// The block scalar holds escaped text, not the secret; the reference
	// spelling "PASSWORD}" is not a leak either.
	if leaks := LiteralSecretLeaks([]byte(out), secrets); len(leaks) != 0 {
		t.Errorf("LiteralSecretLeaks = %v, want none", leaks)
	}
}

// TestLiteralSecretLeaks_FindsEscapedSecret: a secret the YAML encoder
// escaped is reported, which a byte search alone misses.
func TestLiteralSecretLeaks_FindsEscapedSecret(t *testing.T) {
	secrets := map[string]string{"MINIO_ROOT_PASSWORD": "fake\t\"pw\\notreal"}
	doc := marshalEnv(t, map[string]string{"OTHER": "x " + secrets["MINIO_ROOT_PASSWORD"]})
	if strings.Contains(string(doc), secrets["MINIO_ROOT_PASSWORD"]) {
		t.Fatalf("fixture is not escaped:\n%s", doc)
	}
	if leaks := LiteralSecretLeaks(doc, secrets); len(leaks) != 1 {
		t.Errorf("LiteralSecretLeaks = %v, want [MINIO_ROOT_PASSWORD]", leaks)
	}
}

// TestTemplateSecrets_PreservesEveryValue is the invariant behind D-0635:
// for random secrets and env values (holding quotes, backslashes, tabs, '#',
// '$', ':' and '@'), the templated YAML loads, and every value resolves to
// exactly the bytes it held before templating.
func TestTemplateSecrets_PreservesEveryValue(t *testing.T) {
	const alphabet = "abcde '\"\\\t#$:@%/-"
	rng := rand.New(rand.NewSource(635))
	word := func(min, max int) string {
		b := make([]byte, min+rng.Intn(max-min+1))
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return string(b)
	}
	keys := []string{"POSTGRES_PASSWORD", "REDIS_PASSWORD", "MINIO_ROOT_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET"}
	for iter := 0; iter < 3000; iter++ {
		secrets := map[string]string{}
		for _, k := range keys {
			if s := word(1, 12); strings.TrimSpace(s) != "" {
				secrets[k] = s
			}
		}
		names := make([]string, 0, len(secrets))
		for k := range secrets {
			names = append(names, k)
		}
		sort.Strings(names)
		env := map[string]string{}
		for i := 0; i < 6; i++ {
			v := word(0, 6)
			if len(names) > 0 && rng.Intn(3) > 0 {
				pw := secrets[names[rng.Intn(len(names))]]
				if rng.Intn(2) == 0 {
					pw = config.URLPassword(pw)
				}
				v = word(0, 4) + ":" + pw + "@" + word(0, 4)
			}
			env["E"+string(rune('0'+i))] = v
		}
		got, out := templatedEnv(t, marshalEnv(t, env), secrets)
		for k, want := range env {
			if r := resolveSecrets(got[k], secrets); r != want {
				t.Fatalf("iter %d: %s resolves to %q, want %q\nsecrets %q\n%s", iter, k, r, want, secrets, out)
			}
		}
	}
}

// TestTemplateSecrets_MultiLineAndKeyPositions: a multi-line secret on a
// secret-named key loses its whole block, not only the first line (lens
// round 2), and a secret that spells a mapping key (a volume name) does not
// rename it, since compose does not interpolate keys.
func TestTemplateSecrets_MultiLineAndKeyPositions(t *testing.T) {
	secrets := map[string]string{
		"POSTGRES_PASSWORD": "line1-abcdef\nline2$x-longer",
		"MINIO_ROOT_USER":   "postgres-db-data",
	}
	in := "services:\n  app:\n    environment:\n      POSTGRES_PASSWORD: |-\n        line1-abcdef\n        line2$x-longer\n" +
		"      OTHER: kept\n    volumes:\n      - postgres-db-data:/var/lib\nvolumes:\n  postgres-db-data: {}\n"
	env, out := templatedEnv(t, []byte(in), secrets)
	if env["POSTGRES_PASSWORD"] != "${POSTGRES_PASSWORD}" || env["OTHER"] != "kept" {
		t.Errorf("multi-line value not replaced whole:\n%s", out)
	}
	if !strings.Contains(out, "\n  postgres-db-data: {}") || !strings.Contains(out, "- postgres-db-data:/var/lib") {
		t.Errorf("volume key renamed by a secret:\n%s", out)
	}
	if leaks := LiteralSecretLeaks([]byte(out), secrets); len(leaks) != 1 || leaks[0] != "MINIO_ROOT_USER" {
		t.Errorf("LiteralSecretLeaks = %v, want [MINIO_ROOT_USER] (left literal, reported)", leaks)
	}
	// The leak net also catches a secret-named key whose value is not its
	// bare reference (what Phase A used to leave behind).
	broken := "services:\n  app:\n    environment:\n      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}\n        line2-tail\n"
	if leaks := LiteralSecretLeaks([]byte(broken), map[string]string{"POSTGRES_PASSWORD": "x"}); len(leaks) != 1 {
		t.Errorf("LiteralSecretLeaks = %v, want the extended value reported", leaks)
	}
}

// TestTemplateSecrets_KeepsLayout: rewriting a key keeps the blank lines
// after it and the document's final newline.
func TestTemplateSecrets_KeepsLayout(t *testing.T) {
	secrets := map[string]string{"POSTGRES_PASSWORD": "fakepw-notreal-123"}
	cases := map[string]string{
		"services:\n  a:\n    environment:\n      POSTGRES_PASSWORD: x\n":                       "services:\n  a:\n    environment:\n      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}\n",
		"services:\n  a:\n    environment:\n      POSTGRES_PASSWORD: x\n\n  b:\n    image: y\n": "services:\n  a:\n    environment:\n      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}\n\n  b:\n    image: y\n",
		"x:\n  POSTGRES_PASSWORD: |-\n    l1\n\n    l2\n\ny: z\n":                               "x:\n  POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}\n\ny: z\n",
	}
	for in, want := range cases {
		if got := string(TemplateSecrets([]byte(in), secrets)); got != want {
			t.Errorf("TemplateSecrets(%q) =\n%q\nwant\n%q", in, got, want)
		}
	}
}
