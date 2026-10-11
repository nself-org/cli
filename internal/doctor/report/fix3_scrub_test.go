package report

// Tests for fix round 3 (lead ruling R6 d, e and the scrubber items of the Opus
// adversarial review): a known literal must not shield the rest of a secret,
// head-cut PEMs with long log prefixes, \/ JSON and userinfo spellings,
// user-only URL userinfo, scrub-before-cap. Every secret is built at run time.

import (
	"net/url"
	"strings"
	"testing"
)

// M1 / R6 d: the literal pass replaced only the head of a longer secret-named
// value; the later passes must redact the whole token.
func TestScrubKnownLiteralDoesNotShieldTheTail(t *testing.T) {
	u, p := "u"+rep("Kz", 3), "P"+rep("9xQ", 5)
	k, sig := "k"+rep("Wy", 4), "S"+rep("g7T", 6)
	cases := []struct {
		name, key, known, in, secret string
	}{
		{"assignment", "BASIC_AUTH_USER", u, "basic_auth=" + u + ":" + p + "\n", p},
		{"token dot signature", "API_KEY_ID", k, "api_token=" + k + "." + sig + "\n", sig},
		{"curl user", "BASIC_AUTH_USER", u, "+ curl -u " + u + ":" + p + " https://x\n", p},
		{"colon form", "BASIC_AUTH_USER", u, "password: " + u + p + "\n", p},
		{"block scalar", "BASIC_AUTH_USER", u, "SOME_TOKEN: |\n    " + u + p + "\n  OTHER: 1\n", p},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			with := NewScrubber(ExpandSecretValues(c.key, c.known))
			if out := with.Scrub(c.in); strings.Contains(out, c.secret) {
				t.Errorf("tail survived behind the known literal:\n%s", out)
			}
			without := NewScrubber(nil) // control: the pattern passes alone
			if out := without.Scrub(c.in); strings.Contains(out, c.secret) {
				t.Errorf("control leaked:\n%s", out)
			}
		})
	}
}

// R6 d: a token that holds only placeholders and separators stays as it is.
func TestOnlyPlaceholders(t *testing.T) {
	yes := []string{"[REDACTED]", "[REDACTED:A_B]", "[REDACTED:A]:[REDACTED:B]", "[REDACTED:A].[REDACTED]"}
	no := []string{"", "plain", "[REDACTED:A]x", "x[REDACTED:A]", "[REDACTED:A].sig9", "[REDACTED:A]é"}
	for _, s := range yes {
		if !onlyPlaceholders(s) {
			t.Errorf("%q should be only placeholders", s)
		}
	}
	for _, s := range no {
		if onlyPlaceholders(s) {
			t.Errorf("%q should not be only placeholders", s)
		}
	}
	s := NewScrubber(ExpandSecretValues("DB_PASSWORD", "pw"+rep("Jj3", 4)))
	if out := s.Scrub("password=" + "pw" + rep("Jj3", 4) + "\n"); out != "password=[REDACTED:DB_PASSWORD]\n" {
		t.Errorf("a fully redacted value was rewritten: %q", out)
	}
}

// S3: a PEM cut at the head, every body line carrying a Postgres log prefix.
func TestScrubHeadCutPEMWithLongLogPrefix(t *testing.T) {
	pre := "2026-10-11 12:00:00.123 UTC [42] LOG:  "
	b1, b2, b3 := rep("MIIFAKEFAKEFAKE", 4), rep("ZmFrZWZha2VrZXk", 4), rep("QUFBQUFBQUFBQUF", 4)
	in := pre + "checkpoint complete\n" + pre + b1 + "\n" + pre + b2 + "\n" + pre + b3 + "\n" + pre + "-----END " + "PRIVATE KEY-----\n" + pre + "ready\n"
	s := NewScrubber(nil)
	out := s.Scrub(in)
	for _, b := range []string{b1, b2, b3} {
		if strings.Contains(out, b) {
			t.Errorf("PEM body line survived:\n%s", out)
		}
	}
	for _, keep := range []string{"checkpoint complete", "LOG:  ready"} {
		if !strings.Contains(out, keep) {
			t.Errorf("neighbour %q was over-redacted:\n%s", keep, out)
		}
	}
}

// R6 d on the PEM walk: a body line the literal pass hit in the middle must not
// stop the walk, or the lines above it survive.
func TestScrubPEMWalkPassesAPlaceholderLine(t *testing.T) {
	v := "mid" + rep("Zq", 4)
	b1, b3 := rep("MIIFAKEFAKEFAKE", 4), rep("ZmFrZWZha2VrZXk", 4)
	in := b1 + "\nAAAA" + v + "BBBBBBBB\n" + b3 + "\n-----END " + "PRIVATE KEY-----\n"
	out := NewScrubber(ExpandSecretValues("MID_KEY", v)).Scrub(in)
	for _, b := range []string{b1, b3, "BBBBBBBB"} {
		if strings.Contains(out, b) {
			t.Errorf("%q survived:\n%s", b, out)
		}
	}
}

// S4: PHP json_encode writes "/" as \/ ; AWS-style secrets carry / and +.
func TestScrubEscapedSlashJSONVariant(t *testing.T) {
	v := "wJalr" + rep("Xu+7/Kd", 5)
	logged := strings.ReplaceAll(v, "/", `\/`)
	out := NewScrubber(ExpandSecretValues("AWS_SECRET_ACCESS_KEY", v)).Scrub(`{"field":"` + logged + `"}`)
	if strings.Contains(out, logged) || strings.Contains(out, "Xu+7") {
		t.Errorf("the \\/ spelling survived: %s", out)
	}
}

// Lens 2: url.UserPassword leaves ; and , raw, unlike QueryEscape/PathEscape.
func TestScrubUserinfoSpelling(t *testing.T) {
	pw := "Fk;9x,Q/" + rep("Rb4", 4)
	ui := strings.TrimPrefix(url.UserPassword("x", pw).String(), "x:")
	if ui == url.QueryEscape(pw) || ui == url.PathEscape(pw) {
		t.Fatalf("the repro is moot: %q equals another spelling", ui)
	}
	out := NewScrubber(ExpandSecretValues("POSTGRES_PASSWORD", pw)).Scrub("cred " + ui + " end\n")
	if strings.Contains(out, ui) || strings.Contains(out, "Rb4Rb4") {
		t.Errorf("userinfo spelling survived: %s", out)
	}
}

// S5: a token used as the URL username is learned as a literal (20+ chars);
// a short user name such as "postgres" is not.
func TestScrubUserOnlyUserinfo(t *testing.T) {
	tok := "tk" + rep("Hm5", 8)
	url1 := "https://" + tok + "@github.com/o/r.git"
	out := NewScrubber(ExpandSecretValues("REPO_URL", url1)).Scrub("cloning with " + tok + " now\n")
	if strings.Contains(out, tok) {
		t.Errorf("user-only token survived: %s", out)
	}
	short := NewScrubber(ExpandSecretValues("REPO_URL", "postgres://postgres@db:5432/x"))
	if out := short.Scrub("connect as postgres to db\n"); !strings.Contains(out, "as postgres to") {
		t.Errorf("a short user name was learned: %s", out)
	}
}
