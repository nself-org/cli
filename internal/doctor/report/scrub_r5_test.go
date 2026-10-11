package report

// Corpus for security CR r2 (R5 d): licence prefixes, webhook URL tokens, SQL
// passwords, curl credentials and "="-joined flag values with a leading dash.
// Every secret is built at run time. All rows are pattern-only (no known
// value), so only the pattern passes can remove them.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func r5Cases() []scrubCase {
	body := "Tk" + rep("9z", 12)
	pw := "Pw" + rep("7q", 8)
	dash := "-" + rep("Dd5", 6)
	hook := "T" + rep("0A1", 3) + "/B" + rep("2C3", 3) + "/" + rep("x4Y", 8)
	disc := rep("123456789", 2) + "/" + rep("Dc7", 12)
	tg := "123456789:" + rep("Tg6", 12)
	cs := []scrubCase{
		{"slack webhook", false, "POST https://hooks.slack.com/services/" + hook + " failed\n", []string{hook}, []string{"hooks.slack.com/services/", " failed"}},
		{"discord webhook", false, "GET https://discord.com/api/webhooks/" + disc + " 404\nGET https://discordapp.com/api/v10/webhooks/" + disc + " 404\n", []string{disc}, []string{"discord.com/api/webhooks/", " 404"}},
		{"telegram bot", false, "https://api.telegram.org/bot" + tg + "/sendMessage?chat=1\n", []string{tg}, []string{"api.telegram.org/bot", "/sendMessage"}},
		{"sql password", false, "ERROR: ALTER ROLE app WITH PASSWORD '" + pw + "';\nCREATE USER x IDENTIFIED BY \"" + pw + "x\";\nSET password 'it''s" + pw + "' done\n", []string{pw}, []string{"ALTER ROLE app WITH PASSWORD", "IDENTIFIED BY", " done"}},
		{"sql password prose kept", false, "password authentication failed for user app\n", nil, []string{"password authentication failed for user app"}},
		{"curl user", false, "exec: curl -sS -u admin:" + pw + " http://h/x\ncurl --user=admin:" + pw + "2 http://h\n", []string{pw}, []string{"curl -sS -u admin:", "http://h/x"}},
		{"curl user list form", false, `command: ["curl","-u","admin:` + pw + `","http://h"]`, []string{pw}, []string{`"http://h"`}},
		{"non-curl -u kept", false, "docker run -u 1000:1000 img\n", nil, []string{"-u 1000:1000"}},
		{"dash value equals-joined", false, "run --password=" + dash + " --admin-secret='" + dash + "x' --ok\n", []string{dash, dash + "x"}, []string{"--ok"}},
		{"r5 names by pattern", false, "GITHUB_PAT=" + pw + "\nDB_PWD: " + pw + "1\nSLACK_WEBHOOK_URL=" + pw + "2\nSIG_HMAC=" + pw + "3\nrun --pat " + pw + "4 --ok\n", []string{pw}, []string{"--ok"}},
		{"r5 path-like names kept", false, "PATH=/usr/local/bin:/usr/bin\nPATTERN=alphabetical\nXPAT=plainvalue\nXPATH=/a/b\nPATIENT=none-here\n", nil, []string{"PATH=/usr/local/bin:/usr/bin", "PATTERN=alphabetical", "XPAT=plainvalue", "XPATH=/a/b", "PATIENT=none-here"}},
		{"quoted flag value with space", false, `run --password "a b ` + pw + `" --ok` + "\n", []string{pw}, []string{"--ok"}},
	}
	for _, pre := range []string{"owner", "plus", "claw", "clawde", "chat", "media", "family", "pro", "ent", "max"} {
		k := "nself_" + pre + "_" + body
		cs = append(cs, scrubCase{"license prefix " + pre, false, "license check: key " + k + " (HTTP 401) end\n", []string{body}, []string{" end"}})
	}
	return cs
}

func TestScrubR5Corpus(t *testing.T) {
	for _, tc := range r5Cases() {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScrubber(nil)
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

// The licence package keeps its prefix list unexported; this guard fails when
// a prefix is added there but not to licenseKeyRe.
func TestLicensePrefixesMatchLicensePackage(t *testing.T) {
	src, err := os.ReadFile("../../license/keys.go")
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`\{"(nself_[a-z]+_)",`).FindAllStringSubmatch(string(src), -1)
	if len(found) < 10 {
		t.Fatalf("parsed %d prefixes from internal/license/keys.go, want at least 10", len(found))
	}
	for _, m := range found {
		if !licenseKeyRe.MatchString(m[1] + "Ab12Cd34Ef") {
			t.Errorf("licence prefix %s is not covered by licenseKeyRe", m[1])
		}
	}
}
