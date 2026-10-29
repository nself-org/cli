package build

// Purpose: Repro + regression tests for PCI compose-secret-templating
//          (2026-07-03): the generated docker-compose.yml must contain NO
//          literal secret values — only ${VAR} references resolved from
//          .nself/compose.env at container-start time.

import (
	"bytes"
	"fmt"
	"github.com/nself-org/cli/internal/compat/compattest"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

// secretTestConfig returns a config with distinctive secret values so literal
// leaks are unambiguous in assertions.
func secretTestConfig() *config.Config {
	cfg := &config.Config{
		ProjectName:   "sectest",
		BaseDomain:    "localhost",
		Env:           "dev",
		DockerNetwork: "sectest_network",
		Postgres: config.PostgresConfig{
			Host:     "postgres",
			User:     "postgres",
			Password: "pg-secret-Zx9Qw8Er7T",
			DB:       "nself",
			Port:     5432,
			Version:  "16",
		},
		Hasura: config.HasuraConfig{
			AdminSecret: "hasura-admin-K3v9Bn2Mp5",
			JWTKey:      "jwt-key-A1b2C3d4E5f6G7h8I9j0K1l2M3n4O5p6",
			JWTType:     "HS256",
			Port:        8080,
			Version:     "v2.36.0",
			MemLimit:    "1g",
			CPULimit:    "1.0",
		},
		Auth: config.AuthConfig{
			Version:            "0.36.0",
			Port:               4000,
			ClientURL:          "http://localhost:3000",
			AccessTokenExpiry:  900,
			RefreshTokenExpiry: 2592000,
			SMTPHost:           "mailpit",
			SMTPPort:           1025,
			SMTPPass:           "smtp-pass-Qw4Rt6Yu8I",
			SMTPSender:         "noreply@localhost",
			MemLimit:           "256m",
			CPULimit:           "0.25",
		},
	}
	cfg.Minio.Enabled = true
	cfg.Minio.Version = "latest"
	cfg.Minio.Port = 9000
	cfg.Minio.ConsolePort = 9001
	cfg.Minio.RootUser = "minio-user-F7g8H9j0"
	cfg.Minio.RootPassword = "minio-pass-L5m6N7b8V9"
	cfg.Minio.MemLimit = "1G"
	cfg.Minio.CPULimit = "0.5"
	return cfg
}

// TestTemplateSecrets_NoLiteralSecretsInGeneratedCompose is the PCI repro:
// run the real compose generator, apply secret templating, and assert the
// output contains only ${VAR} references — zero literal secret values.
func TestTemplateSecrets_NoLiteralSecretsInGeneratedCompose(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	cfg := secretTestConfig()
	raw, err := compose.NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("compose generation failed: %v", err)
	}

	secrets := SecretEnvMap(cfg)
	templated := TemplateSecrets(raw, secrets)
	doc := string(templated)

	// No secret literal may survive.
	for name, val := range secrets {
		if strings.Contains(doc, val) {
			t.Errorf("literal secret %s (%q) still present in generated compose", name, val)
		}
	}
	if leaks := LiteralSecretLeaks(templated, secrets); len(leaks) != 0 {
		t.Errorf("LiteralSecretLeaks reported %v, want none", leaks)
	}

	// The ${VAR} references must be present instead.
	for _, ref := range []string{
		"POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}",
		"HASURA_GRAPHQL_ADMIN_SECRET: ${HASURA_GRAPHQL_ADMIN_SECRET}",
		"HASURA_GRAPHQL_JWT_SECRET: ${HASURA_GRAPHQL_JWT_SECRET}",
		"MINIO_ROOT_USER: ${MINIO_ROOT_USER}",
		"MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}",
		"AUTH_JWT_SECRET: ${HASURA_JWT_KEY}",
		":${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@", // DATABASE_URL credential position (D-0334)
	} {
		if !strings.Contains(doc, ref) {
			t.Errorf("expected templated reference %q in generated compose", ref)
		}
	}
}

// TestTemplateSecrets_CommandAndHealthcheckLiteralsRewritten is the P6-E2-
// W2-S3-T9 finding: Redis embedded REDIS_PASSWORD in its `command:` string
// (--requirepass) and healthcheck `test:` args, and Typesense embedded
// TYPESENSE_API_KEY in its `command:` string (--api-key=) and healthcheck
// header. Since D-0282 the generator keeps both off argv entirely (Redis
// reads REDISCLI_AUTH, Typesense its TYPESENSE_API_KEY env), so the
// generated compose must hold no literal and the new env-only shape. Phase C
// still has to rewrite a literal a generator puts in a command string; the
// synthetic case below keeps that covered without a live fixture.
func TestTemplateSecrets_CommandAndHealthcheckLiteralsRewritten(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	cfg := secretTestConfig()
	cfg.Redis.Enabled = true
	cfg.Redis.Password = "redis-pass-K9m3Bq7Wz2"
	cfg.Search.Enabled = true
	cfg.Search.Engine = "typesense"
	cfg.Search.Typesense.APIKey = "typesense-key-Vb4Xr8Nq1L"

	raw, err := compose.NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("compose generation failed: %v", err)
	}

	secrets := SecretEnvMap(cfg)
	if _, ok := secrets["TYPESENSE_API_KEY"]; !ok {
		t.Fatal("SecretEnvMap must cover TYPESENSE_API_KEY")
	}

	templated := TemplateSecrets(raw, secrets)
	doc := string(templated)

	for _, literal := range []string{cfg.Redis.Password, cfg.Search.Typesense.APIKey} {
		if strings.Contains(doc, literal) {
			t.Errorf("literal secret %q still present in generated compose (command/healthcheck leak):\n%s", literal, doc)
		}
	}
	if leaks := LiteralSecretLeaks(templated, secrets); len(leaks) != 0 {
		t.Errorf("LiteralSecretLeaks reported %v, want none", leaks)
	}
	for _, ref := range []string{
		"REDISCLI_AUTH: ${REDIS_PASSWORD}",
		`requirepass "$$esc"`,
		"TYPESENSE_API_KEY: ${TYPESENSE_API_KEY}",
	} {
		if !strings.Contains(doc, ref) {
			t.Errorf("expected %q in generated compose:\n%s", ref, doc)
		}
	}
	for _, gone := range []string{"--requirepass ${REDIS_PASSWORD}", "--api-key=", "X-TYPESENSE-API-KEY"} {
		if strings.Contains(doc, gone) {
			t.Errorf("%q is back on a generated argv (D-0282)", gone)
		}
	}

	synthetic := "services:\n  x:\n    command: srv --requirepass " + cfg.Redis.Password + "\n"
	if got := string(TemplateSecrets([]byte(synthetic), secrets)); !strings.Contains(got, "--requirepass ${REDIS_PASSWORD}") {
		t.Errorf("Phase C did not rewrite a command-string literal:\n%s", got)
	}
}

// TestTemplateSecrets_AliasKeyLeftAloneWhenValueDiffers ensures alias env keys
// carrying an independent value are never clobbered.
func TestTemplateSecrets_AliasKeyLeftAloneWhenValueDiffers(t *testing.T) {
	yaml := "services:\n  auth:\n    environment:\n      AUTH_JWT_SECRET: some-other-independent-value\n"
	secrets := map[string]string{"HASURA_JWT_KEY": "jwt-key-A1b2C3d4E5f6"}
	out := string(TemplateSecrets([]byte(yaml), secrets))
	if !strings.Contains(out, "AUTH_JWT_SECRET: some-other-independent-value") {
		t.Errorf("alias key with independent value was clobbered:\n%s", out)
	}
}

func TestWriteComposeEnv_ContentAndPermissions(t *testing.T) {
	workdir := t.TempDir()
	cfg := secretTestConfig()
	secrets := SecretEnvMap(cfg)
	plugins := map[string]string{"NSELF_PLUGIN_DIR": "/tmp/plugins"}

	if err := WriteComposeEnv(workdir, cfg, secrets, plugins); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(workdir, ".nself", "compose.env")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows os.Chmod cannot represent POSIX owner-only bits (NTFS ACLs, not
	// mode bits) — same exception as internal/build/hasura_config_test.go.
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Errorf("compose.env permissions = %o, want 0600", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"POSTGRES_PASSWORD=" + cfg.Postgres.Password,
		"HASURA_GRAPHQL_ADMIN_SECRET=" + cfg.Hasura.AdminSecret,
		"DOCKER_NETWORK=sectest_network",
		"NSELF_PLUGIN_DIR=/tmp/plugins",
		"HASURA_GRAPHQL_ENABLE_CONSOLE=false",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("compose.env missing %q", want)
		}
	}
}

func TestComposeEnvFiles_OrderAndFallback(t *testing.T) {
	// Legacy project: no compose.env → nil (docker compose default discovery).
	legacy := t.TempDir()
	if err := os.WriteFile(filepath.Join(legacy, ".env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ComposeEnvFiles(legacy); got != nil {
		t.Errorf("legacy project: expected nil env files, got %v", got)
	}

	// Templated project: .env + .nself/compose.env, compose.env last (wins).
	templated := t.TempDir()
	if err := os.WriteFile(filepath.Join(templated, ".env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(templated, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templated, ".nself", "compose.env"), []byte("B=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ComposeEnvFiles(templated)
	if len(got) != 2 {
		t.Fatalf("expected 2 env files, got %v", got)
	}
	if filepath.Base(got[0]) != ".env" || filepath.Base(got[1]) != "compose.env" {
		t.Errorf("env file order wrong: %v", got)
	}
}

// urlEncPasswords are test-only values covering every URL-reserved character.
// Assertions never print them: only the case index appears in failures.
var urlEncPasswords = []string{
	"a/b@c:d#e?f%g",
	"p@ss",
	"abc",
	"a+b c",
	"pa$$word$HOME",
	"@:/?#%+ ",
	"100%25",
}

// readComposeEnvMap parses .nself/compose.env the way docker compose reads a
// dotenv file for the values this test cares about: KEY=VALUE per line, one
// layer of single quotes removed, or the double-quoted form
// config.QuoteComposeEnvValue writes for values holding ' or \ undone.
// (internal/compose's parseComposeDotenv is the full compose-go port.)
func readComposeEnvMap(t *testing.T, workdir string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workdir, ".nself", "compose.env"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("malformed compose.env line (key %q)", k)
		}
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			out[k] = strings.NewReplacer(`\\`, `\`, `\"`, `"`, "$$", "$").Replace(v[1 : len(v)-1])
			continue
		}
		out[k] = config.UnquoteEnvValue(v)
	}
	return out
}

// TestWriteComposeEnv_URLEncVars: compose.env carries both encoded twins; the
// parsed twin decodes (url.Parse) back to the original; the raw variables and
// DATABASE_URL are untouched and not double-encoded.
func TestWriteComposeEnv_URLEncVars(t *testing.T) {
	for i, pw := range urlEncPasswords {
		workdir := t.TempDir()
		cfg := secretTestConfig()
		cfg.Postgres.Password = pw
		cfg.Redis.Enabled = true
		cfg.Redis.Password = pw + "r"
		if err := WriteComposeEnv(workdir, cfg, SecretEnvMap(cfg), nil); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		env := readComposeEnvMap(t, workdir)

		for _, c := range []struct{ key, raw string }{
			{"POSTGRES_PASSWORD", pw},
			{"REDIS_PASSWORD", pw + "r"},
		} {
			if env[c.key] != c.raw {
				t.Errorf("case %d: %s changed", i, c.key)
			}
			enc, ok := env[c.key+"_URLENC"]
			if !ok {
				t.Fatalf("case %d: %s_URLENC missing", i, c.key)
			}
			if enc != config.URLPassword(c.raw) {
				t.Errorf("case %d: %s_URLENC is not the single-encoded password", i, c.key)
			}
			u, err := url.Parse("redis://:" + enc + "@redis:6379")
			if err != nil {
				t.Fatalf("case %d: %s_URLENC does not parse in a URL: %v", i, c.key, err)
			}
			if got, _ := u.User.Password(); got != c.raw || u.Host != "redis:6379" {
				t.Errorf("case %d: %s_URLENC does not round-trip", i, c.key)
			}
		}
		// DATABASE_URL was already encoded once by cfg.DatabaseURL(); the twin
		// must not alter or re-encode it.
		du, err := url.Parse(env["DATABASE_URL"])
		if err != nil {
			t.Fatalf("case %d: DATABASE_URL: %v", i, err)
		}
		if got, _ := du.User.Password(); got != pw {
			t.Errorf("case %d: DATABASE_URL password no longer round-trips", i)
		}
	}
}

func TestWriteComposeEnv_URLEncOmittedWhenPasswordEmpty(t *testing.T) {
	workdir := t.TempDir()
	cfg := secretTestConfig()
	cfg.Redis.Password = ""
	if err := WriteComposeEnv(workdir, cfg, SecretEnvMap(cfg), nil); err != nil {
		t.Fatal(err)
	}
	env := readComposeEnvMap(t, workdir)
	if _, ok := env["REDIS_PASSWORD_URLENC"]; ok {
		t.Error("REDIS_PASSWORD_URLENC written for an empty password")
	}
	if _, ok := env["POSTGRES_PASSWORD_URLENC"]; !ok {
		t.Error("POSTGRES_PASSWORD_URLENC missing")
	}
}

// A twin that contains "$" must be single-quoted or compose expands "$name"
// inside the dotenv value and silently truncates the password.
func TestWriteComposeEnv_URLEncDollarIsQuoted(t *testing.T) {
	workdir := t.TempDir()
	cfg := secretTestConfig()
	cfg.Postgres.Password = "pa$$word$HOME"
	if err := WriteComposeEnv(workdir, cfg, SecretEnvMap(cfg), nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(workdir, ".nself", "compose.env"))
	var line string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "POSTGRES_PASSWORD_URLENC=") {
			line = strings.TrimPrefix(l, "POSTGRES_PASSWORD_URLENC=")
		}
	}
	if len(line) < 2 || line[0] != '\'' || line[len(line)-1] != '\'' {
		t.Fatal("a twin containing $ must be single-quoted in compose.env")
	}
	if strings.Contains(line, "'") && strings.Count(line, "'") != 2 {
		t.Fatal("encoded value must not contain a single quote")
	}
}

// TestURLEncConsumersAreDecoders guards the consumers of the encoded URLs:
// a tool that does not percent-decode the password (redis-cli -u is one: it
// answers WRONGPASS to the encoded form) must never be fed a URL built from
// the encoded variable. Shipped compose files and fixtures may run redis-cli
// only with the raw variable via -a, never with a URL or a _URLENC reference.
// The check is line based AND structural: a YAML list or a multi-line scalar
// that spreads redis-cli and -u over several lines is joined before the test
// (redisCLIURLViolations). A shipped file that does not parse as YAML fails.
// Remaining consumers of encoded URLs are decoding libraries: postgres-exporter
// (DATA_SOURCE_NAME, Go net/url) and the plugin services' DATABASE_URL and
// REDIS_URL.
func TestURLEncConsumersAreDecoders(t *testing.T) {
	var files []string
	for _, pat := range []string{
		"../compose/*.yml",
		"testdata/plugin-compose-fixtures/*.yml",
	} {
		m, err := filepath.Glob(pat)
		if err != nil || len(m) == 0 {
			t.Fatalf("no shipped compose files matched %s", pat)
		}
		files = append(files, m...)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range redisCLIURLViolations(filepath.Base(f), data) {
			t.Error(v)
		}
	}
}

const redisCLIURLMsg = "redis-cli must use -a with the raw password variable, not a URL (it does not percent-decode)"

// redisCLIForbidden reports whether joined redis-cli text feeds it a URL or an
// encoded variable: _URLENC, REDIS_URL, redis://, or a standalone -u / --uri.
func redisCLIForbidden(joined string) bool {
	if strings.Contains(joined, "_URLENC") || strings.Contains(joined, "REDIS_URL") || strings.Contains(joined, "redis://") {
		return true
	}
	for _, tok := range strings.Fields(joined) {
		if tok == "-u" || tok == "--uri" || strings.HasPrefix(tok, "--uri=") {
			return true
		}
	}
	return false
}

// redisCLILineViolations is the original per-line check. It cannot see a YAML
// list that puts redis-cli and -u on different lines.
func redisCLILineViolations(name string, data []byte) []string {
	var out []string
	for i, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "redis-cli") {
			continue
		}
		if strings.Contains(line, "_URLENC") || strings.Contains(line, "REDIS_URL") || strings.Contains(line, "redis://") || strings.Contains(line, " -u ") {
			out = append(out, fmt.Sprintf("%s:%d: %s", name, i+1, redisCLIURLMsg))
		}
	}
	return out
}

// redisCLIURLViolations runs the line check and a YAML walk. The walk joins
// the scalar items of a sequence that mentions redis-cli, and normalises the
// whitespace of a scalar (including | and > blocks) that mentions it, then
// applies redisCLIForbidden. Results are "name:line: message", de-duplicated.
func redisCLIURLViolations(name string, data []byte) []string {
	out := redisCLILineViolations(name, data)
	seen := map[string]bool{}
	for _, v := range out {
		seen[v] = true
	}
	add := func(line int) {
		v := fmt.Sprintf("%s:%d: %s", name, line, redisCLIURLMsg)
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return append(out, fmt.Sprintf("%s: not valid YAML, cannot check redis-cli use: %v", name, err))
		}
		walkRedisCLI(&doc, add)
	}
	return out
}

// walkRedisCLI visits every node of n and calls add(line) for each sequence or
// scalar that runs redis-cli with a forbidden URL form.
func walkRedisCLI(n *yaml.Node, add func(line int)) {
	switch n.Kind {
	case yaml.SequenceNode:
		var items []string
		mentions := false
		for _, c := range n.Content {
			if c.Kind == yaml.ScalarNode {
				items = append(items, c.Value)
				if strings.Contains(c.Value, "redis-cli") {
					mentions = true
				}
			}
		}
		if mentions && redisCLIForbidden(strings.Join(items, " ")) {
			add(n.Line)
		}
	case yaml.ScalarNode:
		if strings.Contains(n.Value, "redis-cli") && redisCLIForbidden(strings.Join(strings.Fields(n.Value), " ")) {
			add(n.Line)
		}
	}
	for _, c := range n.Content {
		walkRedisCLI(c, add)
	}
}

// TestRedisCLIURLGuardParsesYAMLLists proves the guard sees redis-cli calls
// that a per-line scan misses.
func TestRedisCLIURLGuardParsesYAMLLists(t *testing.T) {
	cases := []struct {
		name     string
		yaml     string
		flagged  bool
		lineOnly bool // would the old line-only check flag it?
	}{
		{"flow list with -u and a URL", "test: [\"CMD\", \"redis-cli\", \"-u\", \"redis://r:6379\", \"ping\"]\n", true, true},
		{"block list on separate lines", "test:\n  - redis-cli\n  - -u\n  - ${REDIS_URL}\n  - ping\n", true, false},
		{"block list, -u and a plain host", "test:\n  - redis-cli\n  - -u\n  - cache-host\n", true, false},
		{"literal block, -u on the next line", "command: |\n  redis-cli\n  -u ${REDIS_PASSWORD_URLENC}\n", true, false},
		{"folded block, -u on the next line", "command: >\n  redis-cli\n  -u cache-host\n", true, false},
		{"second document in the stream", "a: 1\n---\ntest:\n  - redis-cli\n  - --uri\n  - x\n", true, false},
		{"clean CMD-SHELL with -a", "test: [\"CMD-SHELL\", \"redis-cli -a \\\"$$REDIS_PASSWORD\\\" ping\"]\n", false, false},
		{"REDISCLI_AUTH environment form", "environment:\n  - REDISCLI_AUTH=${REDIS_PASSWORD}\ntest: [\"CMD\", \"redis-cli\", \"ping\"]\n", false, false},
		{"URL on a service that is not redis-cli", "environment:\n  - REDIS_URL=redis://:${REDIS_PASSWORD_URLENC}@redis:6379\n", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := redisCLIURLViolations("synthetic.yml", []byte(c.yaml))
			if (len(got) > 0) != c.flagged {
				t.Fatalf("flagged=%v, want %v: %v", len(got) > 0, c.flagged, got)
			}
			if lo := redisCLILineViolations("synthetic.yml", []byte(c.yaml)); (len(lo) > 0) != c.lineOnly {
				t.Fatalf("line-only check flagged=%v, want %v: %v", len(lo) > 0, c.lineOnly, lo)
			}
		})
	}
	if got := redisCLIURLViolations("bad.yml", []byte("a: [unclosed\n")); len(got) != 1 || !strings.Contains(got[0], "not valid YAML") {
		t.Fatalf("a YAML parse error must be reported, got %v", got)
	}
}

// Custom services keep the MinIO root identity selected by the generator even
// when a distinct application S3 credential pair is configured (D-0639).
func TestTemplateSecretsCustomServiceMinioIdentity(t *testing.T) {
	compattest.Set(t, false)
	cfg := secretTestConfig()
	cfg.Minio.S3AccessKey = "application-access"
	cfg.Minio.S3SecretKey = "application-secret"
	cfg.CustomServices = []config.CustomService{{Name: "worker", Port: 3000, Template: "express-js"}}
	raw, err := compose.NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatal(err)
	}
	templated := TemplateSecrets(raw, SecretEnvMap(cfg))
	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(templated, &doc); err != nil {
		t.Fatal(err)
	}
	env := doc.Services["worker"].Environment
	for key, want := range map[string]string{"S3_ACCESS_KEY": "${MINIO_ROOT_USER}", "S3_SECRET_KEY": "${MINIO_ROOT_PASSWORD}"} {
		if env[key] != want {
			t.Errorf("%s = %q, want %q", key, env[key], want)
		}
	}
}

// A literal secret shaped like a compose reference ("${ABC}") is still a
// literal: it must become ${AUTH_SMTP_PASS}, not an unrelated substitution.
// A real cross-key reference to another mapped secret is preserved.
func TestTemplateSecretsLiteralRefShapedSecret(t *testing.T) {
	cfg := secretTestConfig()
	cfg.Auth.SMTPPass = "${ABC}"
	secrets := SecretEnvMap(cfg)
	raw := "services:\n  auth:\n    environment:\n      AUTH_SMTP_PASS: ${ABC}\n" +
		"  worker:\n    environment:\n      S3_ACCESS_KEY: ${MINIO_ROOT_USER}\n"
	secrets["MINIO_ROOT_USER"] = "minio-root"
	secrets["S3_ACCESS_KEY"] = "application-access"
	out := string(TemplateSecrets([]byte(raw), secrets))
	if !strings.Contains(out, "AUTH_SMTP_PASS: ${AUTH_SMTP_PASS}") {
		t.Errorf("literal ${ABC} SMTP password not templated:\n%s", out)
	}
	if !strings.Contains(out, "S3_ACCESS_KEY: ${MINIO_ROOT_USER}") {
		t.Errorf("cross-key reference not preserved:\n%s", out)
	}
}

// Exercise generated YAML quoting as well as unknown and mapped variable names.
func TestTemplateSecretsGeneratedSMTPReferenceLiterals(t *testing.T) {
	for _, password := range []string{"${ABC}", "${MINIO_ROOT_USER}", "${MINIO_ROOT_USER:-fallback}", "prefix${ABC}"} {
		t.Run(password, func(t *testing.T) {
			cfg := secretTestConfig()
			cfg.Auth.SMTPPass = password
			raw, err := compose.NewGenerator(cfg).Generate()
			if err != nil {
				t.Fatal(err)
			}
			templated := TemplateSecrets(raw, SecretEnvMap(cfg))
			var doc struct {
				Services map[string]struct {
					Environment map[string]string `yaml:"environment"`
				} `yaml:"services"`
			}
			if err := yaml.Unmarshal(templated, &doc); err != nil {
				t.Fatal(err)
			}
			if got := doc.Services["auth"].Environment["AUTH_SMTP_PASS"]; got != "${AUTH_SMTP_PASS}" {
				t.Errorf("SMTP password reference = %q, want ${AUTH_SMTP_PASS}", got)
			}
		})
	}
}

func TestTemplateSecretsLiteralMinioAlias(t *testing.T) {
	for _, key := range []string{"S3_ACCESS_KEY", "S3_SECRET_KEY"} {
		source := "MINIO_ROOT_USER"
		if key == "S3_SECRET_KEY" {
			source = "MINIO_ROOT_PASSWORD"
		}
		ref := "${" + source + "}"
		raw := []byte("environment:\n  " + key + ": '" + ref + "'\n")
		got := string(TemplateSecrets(raw, map[string]string{key: ref, source: "root-value"}))
		if !strings.Contains(got, key+": ${"+key+"}") {
			t.Errorf("literal alias was preserved: %s", got)
		}
	}
}
