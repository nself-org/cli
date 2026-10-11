package report

// Corpus additions for the security CR round 1 (M2, M3, M5, S1, S2, S3). Every
// secret is built at run time; no literal token-shaped string is committed.

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
	"testing"
)

// Secrets used by moreCases, built once.
var (
	blkA, blkB, blkC = "blkA" + rep("Xy2", 5), "blkB" + rep("Pq9", 5), "blkC" + rep("Mn4", 5)
	pwHash           = "pa#ss" + rep("Q1", 4)
	pwQuery          = "pa?ss" + rep("W2", 4)
	pwSlash          = "pa/ss" + rep("E3", 4)
	pwSpace          = "pa ss" + rep("R4", 4)
	pwDSN            = "ds" + rep("Tn5", 4)
	pwNoScheme       = "ns" + rep("Yb6", 4)
	quoted           = `it's-a-"sec"` + rep("U7", 3)
	basicUser        = "bob"
)

// extraValues are the known literals of the new cases.
func extraValues(c corpus) []SecretValue {
	var out []SecretValue
	add := func(k, v string) { out = append(out, ExpandSecretValues(k, v)...) }
	add("BLOCK_TOKEN", blkA+"\n"+blkB)
	add("JWT_SALT", "salt"+rep("Aa8", 4))
	add("APP_SEED", "seed"+rep("Bb8", 4))
	add("BASIC_AUTH", "adm:"+rep("Cc8", 4))
	add("NSELF_LICENSE", "lic"+rep("Dd8", 4))
	add("SESSION_ID", "sess"+rep("Ee8", 4))
	add("COOKIE_VALUE", "cook"+rep("Ff8", 4))
	add("DB_URL_HASH", "postgres://u:"+pwHash+"@db:5432/x")
	add("DB_URL_QUERY", "postgres://u:"+pwQuery+"@db:5432/x")
	add("DB_URL_SLASH", "postgres://u:"+pwSlash+"@db:5432/x")
	add("DB_URL_SPACE", "postgres://u:"+pwSpace+"@db:5432/x")
	add("MYSQL_URL", "u:"+pwDSN+"@tcp(db:3306)/x")
	add("REPLICA_URL", "//u:"+pwNoScheme+"@host/x")
	add("BASIC_URL", "https://"+basicUser+":"+c.pg+"@registry.example.org")
	add("QUOTED_SECRET", quoted)
	add("OV_A_KEY", "abcdefgh")
	add("OV_B_KEY", "defghijkl")
	return out
}

func moreCases(c corpus) []scrubCase {
	salt, seed, auth, lic := "salt"+rep("Aa8", 4), "seed"+rep("Bb8", 4), "adm:"+rep("Cc8", 4), "lic"+rep("Dd8", 4)
	sess, cook := "sess"+rep("Ee8", 4), "cook"+rep("Ff8", 4)
	body := func(s string, n int) string { return rep(s, n) }
	b1, b2, b3 := body("MIIFAKEFAKEFAKE", 4), body("ZmFrZWZha2VrZXk", 4), "QUFBQQ=="
	pemEnd := "-----END " + "PRIVATE KEY-----"
	std := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	return []scrubCase{
		// M2: block scalars.
		{"block scalar patterns", false, "environment:\n  SOME_TOKEN: |\n    " + blkA + "\n    " + blkB + "\n  OTHER_KEY: >-\n    " + blkC + "\n  PORT: 8080\n", []string{blkA, blkB, blkC}, []string{"PORT: 8080"}},
		{"block scalar known multi-line", true, "x:\n      BLOCK_TOKEN: |\n          " + blkA + "\n        " + blkB + "\nNEXT: 1\n", []string{blkA, blkB}, []string{"NEXT: 1"}},
		{"multi-line value lines in a log", true, "svc | " + blkA + "\nsvc | " + blkB + "\nsvc | tail\n", []string{blkA, blkB}, []string{"svc | tail"}},
		{"block scalar list item", false, "env:\n  - SIGN_KEY: |+\n      " + blkA + "\n    OTHER: keepme\n", []string{blkA}, []string{"OTHER: keepme"}},
		{"block scalar non-secret key kept", false, "script: |\n    echo hello\n", nil, []string{"echo hello"}},
		// M3: widened names, known and by pattern.
		{"salt seed in log", true, "salt " + salt + " used; seed " + seed + " too\n", []string{salt, seed}, []string{" used;"}},
		{"auth license in log", true, "basic " + auth + " lic " + lic + " ok\n", []string{auth, lic}, []string{" ok"}},
		{"session cookie in log", true, "sid " + sess + " ck " + cook + " end\n", []string{sess, cook}, []string{" end"}},
		{"widened names by pattern", false, "JWT_SALT=" + salt + "\nAPP_SEED: " + seed + "\nBASIC_AUTH=" + auth + "\nNSELF_LICENSE=" + lic + "\nSESSION_ID=" + sess + "\nCOOKIE_VALUE=" + cook + "\nDB_AUTH=" + c.pg + "\nHTPASSWD_FILE=" + lic + "\nSIGNING_MATERIAL=" + seed + "\n",
			[]string{salt, seed, auth, lic, sess, cook, c.pg}, nil},
		{"auth url survives", false, "AUTH_URL=https://auth.example.org\nAUTH_ENABLED=true\nAUTH_PORT=4000\n", nil, []string{"AUTH_URL=https://auth.example.org", "AUTH_ENABLED=true", "AUTH_PORT=4000"}},
		// M5: URL passwords with raw punctuation.
		{"url hash known", true, "connect postgres://u:" + pwHash + "@db:5432/x refused\n", []string{pwHash, "u:" + pwHash}, []string{"@db:5432/x refused"}},
		{"url query known", true, "connect postgres://u:" + pwQuery + "@db:5432/x refused\n", []string{pwQuery}, []string{"@db:5432/x refused"}},
		{"url slash known", true, "connect postgres://u:" + pwSlash + "@db:5432/x refused\n", []string{pwSlash}, []string{"@db:5432/x refused"}},
		{"url space known", true, "connect postgres://u:" + pwSpace + "@db:5432/x refused\n", []string{pwSpace, rep("R4", 4)}, []string{"@db:5432/x refused"}},
		{"dsn known", true, "dial u:" + pwDSN + "@tcp(db:3306)/x failed\n", []string{pwDSN}, []string{"failed"}},
		{"schemeless known", true, "open //u:" + pwNoScheme + "@host/x now\n", []string{pwNoScheme}, []string{" now"}},
		{"url hash question slash by pattern", false, "a postgres://u:" + pwHash + "@db:5432/x b postgres://u:" + pwQuery + "@db:5432/x c postgres://u:" + pwSlash + "@db/x d\n",
			[]string{pwHash, pwQuery, pwSlash}, []string{"@db:5432/x b", "@db/x d"}},
		{"schemeless by pattern", false, "open //u:" + pwNoScheme + "@host/x and u:" + pwDSN + "@tcp(db:3306)/x\n", []string{pwNoScheme, pwDSN}, []string{"@host/x and", "@tcp(db:3306)/x"}},
		// S1: PEM tails with log prefixes and JSON wrapping.
		{"pem tail prefixed", false, "svc-1  | start\nsvc-1  | " + b1 + "\nsvc-1  | " + b2 + "\nsvc-1  | " + b3 + "\nsvc-1  | " + pemEnd + "\nsvc-1  | after\n",
			[]string{"MIIFAKE", "ZmFrZWZha2V", b3}, []string{"svc-1  | start", "svc-1  | after"}},
		{"pem tail json lines", false, `{"log":"` + b1 + `\n","stream":"stdout"}` + "\n" + `{"log":"` + b2 + `\n","stream":"stdout"}` + "\n" + `{"log":"` + b3 + `\n","stream":"stdout"}` + "\n" + `{"log":"` + pemEnd + `\n","stream":"stdout"}` + "\n" + `{"log":"after\n","stream":"stdout"}`,
			[]string{"MIIFAKE", "ZmFrZWZha2V", b3}, []string{`{"log":"after\n"`}},
		{"pem any label", false, "-----BEGIN " + "PGP MESSAGE-----\n" + b1 + "\n-----END " + "PGP MESSAGE-----\nok\n", []string{"MIIFAKE"}, []string{"ok"}},
		// S2: encoded forms of a known value.
		{"base64 forms", true, "a=" + std(c.admin) + " b=" + base64.RawStdEncoding.EncodeToString([]byte(c.admin)) + " c=" + base64.URLEncoding.EncodeToString([]byte(c.admin)) + " d=" + base64.RawURLEncoding.EncodeToString([]byte(c.admin)) + " e\n",
			[]string{std(c.admin)[:20], base64.RawURLEncoding.EncodeToString([]byte(c.admin))[:20]}, []string{" e"}},
		{"hex form", true, "h=" + hex.EncodeToString([]byte(c.admin)) + " f\n", []string{hex.EncodeToString([]byte(c.admin))}, []string{" f"}},
		{"shell quote form", true, "sh -c 'echo " + strings.ReplaceAll(quoted, "'", `'\''`) + "' g\n", []string{`'\''`, rep("U7", 3)}, []string{" g"}},
		{"basic auth of url credentials", true, "Authorization-less: " + std(basicUser+":"+c.pg) + " h\n", []string{std(basicUser + ":" + c.pg)}, []string{" h"}},
		// S3: overlapping literals leave no tail.
		{"overlapping literals", true, "x abcdefghijkl y\n", []string{"ijkl", "abcdefgh", "defghijkl", "efghi"}, []string{" y"}},
		{"url escaped overlap", true, "z " + url.QueryEscape(c.tricky) + c.tricky + " w\n", []string{c.tricky, "%20"}, []string{" w"}},
	}
}

func TestSecretNameDefinition(t *testing.T) {
	for _, n := range []string{"JWT_SALT", "APP_SEED", "BASIC_AUTH", "BASICAUTH", "x_basic_auth", "NSELF_LICENSE", "NSELF_LICENCE", "SESSION_ID", "COOKIE_NAME", "SIGNING_KEY", "HTPASSWD", "DB_AUTH", "db_auth", "HASURA_GRAPHQL_ADMIN_SECRET", "SENTRY_DSN"} {
		if !IsSecretName(n) {
			t.Errorf("%s should be secret-like", n)
		}
	}
	for _, n := range []string{"AUTH_URL", "AUTH", "AUTH_ENABLED", "AUTHOR", "POSTGRES_DB", "PORT", "AUTH_HOST"} {
		if IsSecretName(n) {
			t.Errorf("%s should not be secret-like", n)
		}
	}
}

func TestURLCredentialsLenient(t *testing.T) {
	cases := []struct{ key, val, user, pass string }{
		{"DATABASE_URL", "postgres://u:" + pwHash + "@db/x", "u", pwHash},
		{"DATABASE_URL", "postgres://u:" + pwSpace + "@db/x", "u", pwSpace},
		{"X", "redis://:" + pwQuery + "@cache:6379", "", pwQuery},
		{"MYSQL_URL", "u:" + pwDSN + "@tcp(db:3306)/x", "u", pwDSN},
		{"X", "u:" + pwDSN + "@tcp(db:3306)/x", "", ""}, // schemeless, plain key: not learned
		{"PLAIN_URL", "https://host.example.org/@user", "", ""},
	}
	for _, tc := range cases {
		got := urlCredentials(tc.key, tc.val)
		if tc.pass == "" {
			if len(got) != 0 {
				t.Errorf("%s=%s: learned %v", tc.key, tc.val, got)
			}
			continue
		}
		found := false
		for _, g := range got {
			found = found || (g.pass == tc.pass && g.user == tc.user)
		}
		if !found {
			t.Errorf("%s=%s: got %v, want user %q pass %q", tc.key, tc.val, got, tc.user, tc.pass)
		}
	}
}
