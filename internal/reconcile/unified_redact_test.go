package reconcile

import (
	"strings"
	"testing"
)

// TestUnifiedRedactsSecretsInNonEnvFiles drives Unified over compose-shaped
// artifacts. Each case's raw input holds the fake secret (positive control),
// the output must not, and the keep lines must still print.
func TestUnifiedRedactsSecretsInNonEnvFiles(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		before  string
		after   string
		secrets []string // must appear in the raw input, never in the diff
		keep    []string // must appear in the diff as written
		want    []string // redacted forms that must appear
	}{
		{
			name:    "map form",
			path:    "docker-compose.yml",
			before:  "services:\n  hasura:\n    image: hasura/graphql-engine:v2\n    environment:\n      HASURA_GRAPHQL_ADMIN_SECRET: fake-secret-0000\n      HASURA_GRAPHQL_ENABLE_CONSOLE: \"true\"\n",
			after:   "services:\n  hasura:\n    image: hasura/graphql-engine:v2\n    environment:\n      HASURA_GRAPHQL_ADMIN_SECRET: fake-secret-1111\n      HASURA_GRAPHQL_ENABLE_CONSOLE: \"true\"\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			keep:    []string{"     image: hasura/graphql-engine:v2\n", "HASURA_GRAPHQL_ENABLE_CONSOLE: \"true\""},
			want:    []string{"-      HASURA_GRAPHQL_ADMIN_SECRET: [REDACTED]\n", "+      HASURA_GRAPHQL_ADMIN_SECRET: [REDACTED]\n"},
		},
		{
			name:    "list form",
			path:    "docker-compose.yml",
			before:  "    environment:\n      - HASURA_GRAPHQL_ADMIN_SECRET=fake-secret-0000\n      - TZ=UTC\n",
			after:   "    environment:\n      - HASURA_GRAPHQL_ADMIN_SECRET=fake-secret-1111\n      - TZ=UTC\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			keep:    []string{"      - TZ=UTC\n"},
			want:    []string{"-      - HASURA_GRAPHQL_ADMIN_SECRET=[REDACTED]\n"},
		},
		{
			name:    "quoted list form",
			path:    "docker-compose.yml",
			before:  "    environment:\n      - \"HASURA_GRAPHQL_ADMIN_SECRET=fake-secret-0000\"\n      - \"TZ=UTC\"\n",
			after:   "    environment:\n      - \"HASURA_GRAPHQL_ADMIN_SECRET=fake-secret-1111\"\n      - \"TZ=UTC\"\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			keep:    []string{"      - \"TZ=UTC\"\n"},
			want:    []string{"+      - \"HASURA_GRAPHQL_ADMIN_SECRET=[REDACTED]\"\n"},
		},
		{
			name:    "list value with comma and space",
			path:    "docker-compose.yml",
			before:  "    environment:\n      - API_PASSWORD=fake-secret-0000,tail-0000 more-0000\n      - TZ=UTC\n",
			after:   "    environment:\n      - API_PASSWORD=fake-secret-1111,tail-1111 more-1111\n      - TZ=UTC\n",
			secrets: []string{"fake-secret-0000", "tail-0000", "more-0000", "tail-1111"},
			keep:    []string{"      - TZ=UTC\n"},
			want:    []string{"-      - API_PASSWORD=[REDACTED]\n"},
		},
		{
			name:    "requirepass command",
			path:    "docker-compose.yml",
			before:  "    command: redis-server --appendonly yes --requirepass fake-secret-0000 --maxclients 50\n",
			after:   "    command: redis-server --appendonly yes --requirepass fake-secret-1111 --maxclients 50\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			want:    []string{"--appendonly yes --requirepass [REDACTED] --maxclients 50"},
		},
		{
			name:    "requirepass list command",
			path:    "docker-compose.yml",
			before:  "    command: [\"redis-server\", \"--requirepass\", \"fake-secret-0000\"]\n    image: redis:7\n",
			after:   "    command: [\"redis-server\", \"--requirepass\", \"fake-secret-1111\"]\n    image: redis:7\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			want:    []string{"--requirepass\", \"[REDACTED]"},
		},
		{
			name:    "requirepass split over lines",
			path:    "docker-compose.yml",
			before:  "    command:\n      - redis-server\n      - --requirepass\n      - fake-secret-0000\n      - --appendonly\n",
			after:   "    command:\n      - redis-server\n      - --requirepass\n      - fake-secret-1111\n      - --appendonly\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			keep:    []string{"      - --appendonly\n"},
			want:    []string{"      - [REDACTED]\n"},
		},
		{
			name:    "database url credentials",
			path:    "docker-compose.yml",
			before:  "    environment:\n      DATABASE_URL: postgres://fakeuser:fake-secret-0000@db:5432/app\n",
			after:   "    environment:\n      DATABASE_URL: postgres://fakeuser:fake-secret-1111@db:5432/app\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			want:    []string{"-      DATABASE_URL: postgres://****@db:5432/app\n"},
		},
		{
			name:    "url under a plain key",
			path:    "docker-compose.yml",
			before:  "    x-upstream: postgres://fakeuser:fake-secret-0000@db:5432/app\n",
			after:   "    x-upstream: postgres://fakeuser:fake-secret-1111@db:5432/app\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			want:    []string{"x-upstream: postgres://****@db:5432/app"},
		},
		{
			name:    "block scalar",
			path:    "docker-compose.yml",
			before:  "    environment:\n      PRIVATE_KEY: |\n        fake-secret-0000-line1\n        fake-secret-0000-line2\n\n        fake-secret-0000-line3\n      SIBLING: keep-me\n",
			after:   "    environment:\n      PRIVATE_KEY: |\n        fake-secret-1111-line1\n        fake-secret-1111-line2\n\n        fake-secret-1111-line3\n      SIBLING: keep-me\n",
			secrets: []string{"fake-secret-0000-line1", "fake-secret-0000-line3", "fake-secret-1111-line2"},
			keep:    []string{"      SIBLING: keep-me\n"},
			want:    []string{"       PRIVATE_KEY: [REDACTED]\n", "-        [REDACTED]\n", "+        [REDACTED]\n"},
		},
		{
			name:    "multi-line quoted value",
			path:    "config.yaml",
			before:  "api_key: \"fake-secret-0000\nfake-secret-0000-cont\"\nname: keep-me\n",
			after:   "api_key: \"fake-secret-1111\nfake-secret-1111-cont\"\nname: keep-me\n",
			secrets: []string{"fake-secret-0000-cont", "fake-secret-1111"},
			keep:    []string{" name: keep-me\n"},
			want:    []string{"-api_key: [REDACTED]\n", "-[REDACTED]\n"},
		},
		{
			name:    "json pair",
			path:    "settings.json",
			before:  "{\n  \"name\": \"keep-me\",\n  \"adminSecret\": \"fake-secret-0000\",\n  \"inline\": {\"api_token\": \"fake-secret-0000\", \"n\": 1}\n}\n",
			after:   "{\n  \"name\": \"keep-me\",\n  \"adminSecret\": \"fake-secret-1111\",\n  \"inline\": {\"api_token\": \"fake-secret-1111\", \"n\": 1}\n}\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			keep:    []string{"  \"name\": \"keep-me\",\n", "\"n\": 1}"},
			want:    []string{"\"adminSecret\": [REDACTED],\n", "{\"api_token\": \"[REDACTED]\", \"n\": 1}"},
		},
		{
			name:    "shell export",
			path:    "run.sh",
			before:  "export ADMIN_PASSWORD=fake-secret-0000\necho done\n",
			after:   "export ADMIN_PASSWORD=fake-secret-1111\necho done\n",
			secrets: []string{"fake-secret-0000", "fake-secret-1111"},
			keep:    []string{" echo done\n"},
			want:    []string{"-export ADMIN_PASSWORD=[REDACTED]\n"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
			for _, w := range append(append([]string{}, tc.keep...), tc.want...) {
				if !strings.Contains(got, w) {
					t.Errorf("diff lacks %q:\n%s", w, got)
				}
			}
		})
	}
}

// TestUnifiedKeepsNonSecretLines proves a diff of plain compose keys prints
// exactly as written, with a unified-diff shape.
func TestUnifiedKeepsNonSecretLines(t *testing.T) {
	before := "services:\n  web:\n    image: nginx:1.25\n    ports:\n      - \"8080:80\"\n    environment:\n      HASURA_GRAPHQL_ENABLE_CONSOLE: \"true\"\n"
	after := strings.Replace(before, "nginx:1.25", "nginx:1.26", 1)
	got := Unified("docker-compose.yml", []byte(before), []byte(after))
	for _, w := range []string{"-    image: nginx:1.25\n", "+    image: nginx:1.26\n", "       - \"8080:80\"\n"} {
		if !strings.Contains(got, w) {
			t.Errorf("diff lacks %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, redactedValue) {
		t.Errorf("non-secret diff holds a redaction:\n%s", got)
	}
}

// TestRedactSecretLinesKeepsLineCount guards the hunk indexing: redaction maps
// lines one to one.
func TestRedactSecretLinesKeepsLineCount(t *testing.T) {
	in := []string{"a: 1\n", "KEY: |\n", "  x\n", "\n", "b: 2", "cmd: --requirepass\n", "- next\n"}
	out := redactSecretLines(in)
	if len(out) != len(in) {
		t.Fatalf("got %d lines, want %d", len(out), len(in))
	}
	if out[4] != "b: 2" {
		t.Errorf("a missing final newline was added: %q", out[4])
	}
}
