package report

// Tests for the scrubber. Every secret below is synthetic and built at run
// time, so no file in the repo holds a token-shaped literal.

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// scrubCase is one TestScrubCorpus row.
type scrubCase struct {
	name    string
	known   bool // scrubber knows the values (value pass); false: patterns only
	in      string
	absent  []string
	present []string
}

func rep(s string, n int) string { return strings.Repeat(s, n) }

// corpus holds the fake secrets used across the tests.
type corpus struct {
	admin, pg, jwt, ghp, stripe, aws, slack, pem, tricky, short, long string
}

func newCorpus() corpus {
	pemHead, pemEnd := "-----BEGIN "+"PRIVATE KEY-----", "-----END "+"PRIVATE KEY-----"
	return corpus{
		admin:  "adm" + rep("Qx7", 8),
		pg:     "pg" + rep("Zk3", 6),
		jwt:    "ey" + "J" + rep("h", 20) + "." + rep("p", 24) + "." + rep("s", 24),
		ghp:    "gh" + "p_" + rep("F", 36),
		stripe: "sk_" + "live_" + rep("B", 24),
		aws:    "AK" + "IA" + rep("Q", 16),
		slack:  "xo" + "xb-" + rep("1", 12) + "-" + rep("a", 20),
		pem:    pemHead + "\n" + rep("MIIFAKEFAKEFAKE", 8) + "\n" + rep("ZmFrZWZha2U", 6) + "\n" + pemEnd,
		tricky: `p@ss/w:rd&x=1 "q" a+b%c`,
		short:  "abc",
		long:   "supersecret-extended-tail9",
	}
}

func (c corpus) values() []SecretValue {
	var out []SecretValue
	for k, v := range map[string]string{
		"HASURA_GRAPHQL_ADMIN_SECRET": c.admin, "POSTGRES_PASSWORD": c.pg, "WEBHOOK_TOKEN": c.tricky,
		"META_KEY": "a.b*c+d?(e)[f]{g}|h^$i", "SHORT_SECRET": c.short, "BASE_KEY": "supersecret",
		"EXT_KEY": c.long, "PEM_PRIVATE": "multi\nline\nvalue123",
	} {
		if sv, ok := NewSecretValue(k, v); ok {
			out = append(out, sv)
		}
	}
	out = append(out, extraValues(c)...)
	return out
}

func TestScrubCorpus(t *testing.T) {
	c := newCorpus()
	esc := func(f func(string) string) string { return f(c.tricky) }
	jsonTricky, _ := json.Marshal(c.tricky)
	neighbours := "POSTGRES_DB=myapp\nPORT=5432\nENV=dev\nHASURA_GRAPHQL_ENABLE_CONSOLE=true\n"
	cases := []scrubCase{
		{"env lines", true, neighbours + "HASURA_GRAPHQL_ADMIN_SECRET=" + c.admin + "\nPOSTGRES_PASSWORD=" + c.pg + "\n", []string{c.admin, c.pg}, []string{"POSTGRES_DB=myapp", "PORT=5432", "ENV=dev"}},
		{"env lines patterns only", false, neighbours + "HASURA_GRAPHQL_ADMIN_SECRET=" + c.admin + "\nPOSTGRES_PASSWORD=" + c.pg + "\n", []string{c.admin, c.pg}, []string{"POSTGRES_DB=myapp", "PORT=5432"}},
		{"env quoted", false, `JWT_SECRET="` + c.admin + `"` + "\nAPI_KEY='" + c.pg + "'\n", []string{c.admin, c.pg}, []string{`JWT_SECRET="[REDACTED]"`}},
		{"compose list form", false, "    environment:\n      - POSTGRES_PASSWORD=" + c.pg + "\n      - POSTGRES_DB=myapp\n", []string{c.pg}, []string{"POSTGRES_DB=myapp"}},
		{"compose map form", false, "    environment:\n      POSTGRES_PASSWORD: " + c.pg + "\n      POSTGRES_DB: myapp\n", []string{c.pg}, []string{"POSTGRES_DB: myapp"}},
		{"compose map form known", true, "      HASURA_GRAPHQL_ADMIN_SECRET: " + c.admin + "\n      PORT: 5432\n", []string{c.admin}, []string{"PORT: 5432"}},
		{"compose ref kept", true, "      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}\n      X_TOKEN: \"${X_TOKEN}\"\n", nil, []string{"${POSTGRES_PASSWORD}", "${X_TOKEN}"}},
		{"admin secret echoed in log", true, "2026-10-10T10:00:00Z hasura: admin secret " + c.admin + " rejected for user\nlistening on port 8080\n", []string{c.admin}, []string{"listening on port 8080"}},
		{"admin secret key-value log", false, "hasura: admin_secret: " + c.admin + " rejected\nok line\n", []string{c.admin}, []string{"ok line"}},
		{"hasura header", false, "curl -H 'x-hasura-admin-secret: " + c.admin + "' http://hasura:8080/v1/graphql\n", []string{c.admin}, []string{"http://hasura:8080/v1/graphql"}},
		{"hasura header json", false, `{"x-hasura-admin-secret":"` + c.admin + `","x-hasura-role":"admin"}`, []string{c.admin}, []string{`"x-hasura-role":"admin"`}},
		{"authorization bearer", false, "Authorization: Bearer " + c.pg + "\nGET /healthz 200\n", []string{c.pg}, []string{"GET /healthz 200"}},
		{"authorization basic", false, "authorization: Basic " + c.pg + "==\n", []string{c.pg}, nil},
		{"lowercase bearer", false, "sent bearer " + c.pg + " to upstream\n", []string{c.pg}, []string{"to upstream"}},
		{"cookie header", false, "Cookie: session=" + c.pg + "; theme=dark\nok\n", []string{c.pg}, []string{"ok"}},
		{"pem block", false, "before\n" + c.pem + "\nafter\n", []string{"MIIFAKE", "ZmFrZWZha2U", "PRIVATE KEY-----"}, []string{"before", "after"}},
		{"pem truncated", false, "tail start\n" + strings.SplitN(c.pem, "\n", 2)[0] + "\n" + rep("MIIFAKEFAKE", 10) + "\n", []string{"MIIFAKE"}, []string{"tail start"}},
		{"pem tail only", false, rep("MIIFAKEFAKEFAKE", 8) + "\n-----END " + "RSA PRIVATE KEY-----\nnext line\n", []string{"MIIFAKE"}, []string{"next line"}},
		{"pem escaped newlines", false, `{"key":"-----BEGIN ` + `PRIVATE KEY-----\n` + rep("MIIFAKE", 8) + `\n-----END ` + `PRIVATE KEY-----\n","n":1}`, []string{"MIIFAKE"}, []string{`"n":1`}},
		{"url postgres", false, "DATABASE_URL=postgres://nself:" + c.pg + "@postgres:5432/myapp\n", []string{c.pg}, []string{"@postgres:5432/myapp"}},
		{"url schemes", false, "redis://:" + c.pg + "@cache:6379 amqp://u:" + c.pg + "@mq:5672 https://u:" + c.pg + "@example.org/x mongodb+srv://u:" + c.pg + "@cluster0.example.org/db", []string{c.pg}, []string{"@cache:6379", "@mq:5672", "@example.org/x", "@cluster0.example.org/db"}},
		{"url password with at", false, "postgres://u:p@ss" + c.pg + "@host:5432/db", []string{c.pg, "p@ss"}, []string{"@host:5432/db"}},
		{"url query not userinfo", false, "https://example.org/path?page=2&email=ops\n", nil, []string{"https://example.org/path?page=2&email=ops"}},
		{"query token", false, "GET /cb?token=" + c.pg + "&page=2 HTTP/1.1\n", []string{c.pg}, []string{"page=2"}},
		{"jwt", false, "token seen " + c.jwt + " here\n", []string{c.jwt, "ey" + "J" + rep("h", 20)}, []string{"here"}},
		{"api prefixes", false, strings.Join([]string{c.ghp, c.stripe, c.aws, c.slack}, " | ") + "\nend", []string{c.ghp, c.stripe, c.aws, c.slack}, []string{"end"}},
		{"url query escaped", true, "redirect=" + esc(url.QueryEscape) + " next\n", []string{esc(url.QueryEscape), c.tricky}, []string{" next"}},
		{"url path escaped", true, "/x/" + esc(url.PathEscape) + "/y\n", []string{esc(url.PathEscape)}, []string{"/y"}},
		{"url lowercase hex", true, "/x/" + strings.ToLower(esc(url.QueryEscape)) + "/y\n", []string{strings.ToLower(esc(url.QueryEscape))}, []string{"/y"}},
		{"json escaped", true, `{"msg":"got ` + strings.Trim(string(jsonTricky), `"`) + `","ok":true}`, []string{strings.Trim(string(jsonTricky), `"`)}, []string{`"ok":true`}},
		{"regex metachars", true, "key is a.b*c+d?(e)[f]{g}|h^$i end; axb stays\n", []string{"a.b*c+d?(e)[f]{g}|h^$i"}, []string{"end; axb stays"}},
		{"substring longest first", true, "x " + c.long + " y supersecret z\n", []string{"extended-tail9", c.long, "supersecret"}, []string{" y ", " z"}},
		{"multi-line value one line", true, `v=multi\nline\nvalue123 end`, []string{`multi\nline\nvalue123`}, []string{"end"}},
		{"short value not a literal input", true, "the abc of it\n", nil, []string{"the abc of it"}},
		{"flags space and equals", false, "run --password " + c.pg + " --admin-secret=" + c.admin + " --verbose\n", []string{c.pg, c.admin}, []string{"--verbose"}},
		{"flags list form", false, `command: ["serve","--admin-secret","` + c.admin + `","--port","8080"]`, []string{c.admin}, []string{`"--port"`, `"8080"`}},
		{"nginx directives", false, "proxy_set_header X-Hasura-Admin-Secret " + c.admin + ";\nproxy_set_header Host $host;\n", []string{c.admin}, []string{"proxy_set_header Host $host;"}},
		{"json assignments", false, `{"api_key": "` + c.ghp + `", "name": "web", "dsn":"` + c.pg + `"}`, []string{c.ghp, c.pg}, []string{`"name": "web"`}},
		{"placeholder untouched", true, "KEY=[REDACTED:META_KEY] and TOKEN: [REDACTED]\n", nil, []string{"KEY=[REDACTED:META_KEY]", "TOKEN: [REDACTED]"}},
		{"words kept", false, "DEBUG_TOKEN=true\nPASSWORD_LOGIN: false\n", nil, []string{"DEBUG_TOKEN=true", "PASSWORD_LOGIN: false"}},
	}
	cases = append(cases, moreCases(c)...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s *Scrubber
			if tc.known {
				s = NewScrubber(c.values())
			} else {
				s = NewScrubber(nil)
			}
			out := s.Scrub(tc.in)
			for _, a := range tc.absent {
				if strings.Contains(out, a) {
					t.Errorf("secret %q survived:\n%s", a, out)
				}
			}
			for _, p := range tc.present {
				if !strings.Contains(out, p) {
					t.Errorf("neighbour %q was over-redacted:\n%s", p, out)
				}
			}
			if len(tc.absent) > 0 && s.Total() == 0 {
				t.Error("redaction counts are zero")
			}
		})
	}
}

func TestScrubCountsByClass(t *testing.T) {
	c := newCorpus()
	s := NewScrubber(c.values())
	in := "a " + c.admin + "\n" + c.pem + "\npostgres://u:" + c.pg + "@h/db\nJWT " + c.jwt + "\nMY_PASS=" + "zz" + rep("y", 8) + "\n"
	s.Scrub(in)
	got := s.Counts()
	for _, class := range []string{ClassValue, ClassPEM, ClassUserinfo, ClassPattern, ClassAssignment} {
		if got[class] == 0 {
			t.Errorf("class %s not counted: %v", class, got)
		}
	}
}

func TestScrubValueFilters(t *testing.T) {
	if _, ok := NewSecretValue("PORT", "5432222"); ok {
		t.Error("non-secret name accepted")
	}
	if _, ok := NewSecretValue("DB_PASSWORD", "abc"); ok {
		t.Error("short value accepted")
	}
	if _, ok := NewSecretValue("DB_PASSWORD", "${DB_PASSWORD}"); ok {
		t.Error("reference accepted")
	}
	sv, ok := NewSecretValue("DB_PASSWORD", `"quoted-value-1"`)
	if !ok || sv.Variants[0] != "quoted-value-1" {
		t.Errorf("quotes not trimmed: %+v", sv)
	}
}
