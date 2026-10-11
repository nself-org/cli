package report

// codex_prem_test.go: regression tests for the Codex pre-merge review of
// P7-LIVE-14 (7 items) and lead ruling R9. Every secret is built at run time
// from fake pieces; none is a real value.

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// Item 1: a literal that spells part of a PEM marker (KEY_FORMAT=OPENSSH)
// must not stop the block from being recognised.
func TestCodexPEMMarkerSurvivesLiteralPass(t *testing.T) {
	body := rep("QUJDREVGR0hJSktM", 6)
	pem := "-----BEGIN " + "OPENSSH PRIVATE KEY-----\n" + body + "\n" + body + "\n-----END " + "OPENSSH PRIVATE KEY-----"
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "KEY_FORMAT=OPENSSH\n")
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(map[string]string{"app": "loaded key\n" + pem + "\ndone\n"}))
	assertAbsent(t, files, body)
	// the same through the scrubber alone, with a one-word label
	sv, _ := NewSecretValue("KEY_FORMAT", "OPENSSH")
	out := NewScrubber([]SecretValue{sv}).Scrub(pem)
	if strings.Contains(out, body) {
		t.Errorf("PEM body survived: %q", out)
	}
}

// Item 2 / R9: a value from an env input stays a literal even when it equals a
// structure name; a compose default that equals one is still dropped.
func TestCodexStructureNameFromEnvInputStaysLiteral(t *testing.T) {
	compose := "services:\n  postgres:\n    image: postgres:16-alpine\n"
	cases := []struct {
		name, env string
		environ   []string
	}{
		{"dotenv", "POSTGRES_PASSWORD=postgres\n", nil},
		{"process env", "", []string{"POSTGRES_PASSWORD=postgres"}},
	}
	for _, c := range cases {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".env"), c.env)
		writeFile(t, filepath.Join(dir, "docker-compose.yml"), compose)
		environ := c.environ
		files := collectFiles(t, Options{ProjectDir: dir, Environ: func() []string { return environ }},
			logsSource(map[string]string{"postgres": "client says hello postgres\n"}))
		if strings.Contains(files["logs/postgres.log"], "postgres") {
			t.Errorf("%s: env-sourced password equal to a service name survived: %q", c.name, files["logs/postgres.log"])
		}
	}
}

// R9: the monitoring default keeps the names while the real value is redacted.
func TestCodexStructureDefaultKeptRealValueRedacted(t *testing.T) {
	dir, fragDir := t.TempDir(), t.TempDir()
	real := "r" + rep("Qz7", 5)
	writeFile(t, filepath.Join(dir, ".env"), "POSTGRES_PASSWORD="+real+"\n")
	frag := filepath.Join(fragDir, "docker-compose.plugin.yml")
	writeFile(t, frag, "services:\n  postgres:\n    image: postgres:16-alpine\n    environment:\n      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-postgres}\n")
	files := collectFiles(t, Options{ProjectDir: dir, ComposeFragments: []string{frag}, Environ: noEnviron},
		logsSource(map[string]string{"postgres": "postgres ready; image postgres:16-alpine; pw " + real + "\n"}))
	got := files["logs/postgres.log"]
	if !strings.Contains(got, "postgres ready; image postgres:16-alpine") {
		t.Errorf("structure names were redacted: %q", got)
	}
	assertAbsent(t, files, real)
}

// Item 3: nginx Authorization header values in every directive and quote form.
func TestCodexNginxAuthorizationHeader(t *testing.T) {
	cred := base64.StdEncoding.EncodeToString([]byte("svc" + rep("x9", 6) + ":" + rep("Pw3", 5)))
	forms := []string{
		`proxy_set_header Authorization "Basic ` + cred + `";`,
		`proxy_set_header Authorization 'Basic ` + cred + `';`,
		`proxy_set_header Authorization Basic ` + cred + `;`,
		`proxy_set_header "Authorization" "Basic ` + cred + `";`,
		`proxy_set_header Proxy-Authorization "Basic ` + cred + `";`,
		`add_header authorization "Bearer ` + cred + `";`,
		`more_set_input_headers 'Authorization: Basic ` + cred + `';`,
		`    proxy_set_header   AUTHORIZATION   "Basic ` + cred + `"  ;`,
	}
	for _, f := range forms {
		if out := NewScrubber(nil).Scrub("location / {\n" + f + "\n}\n"); strings.Contains(out, cred) {
			t.Errorf("credential survived in %q: %q", f, out)
		}
	}
	keep := "proxy_set_header Authorization $http_authorization;"
	if out := NewScrubber(nil).Scrub(keep); out != keep {
		t.Errorf("a pass-through variable was rewritten: %q", out)
	}
}

// Item 4: a resolved value that looks like a variable reference is a literal
// when it comes from an env input (process env, single-quoted dotenv); a
// compose-file reference is still not one.
func TestCodexDollarLeadingEnvValuesStayLiteral(t *testing.T) {
	v := "$" + rep("aBcD", 4)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "SQ_ADMIN_SECRET='"+v+"x'\nPLAIN_API_TOKEN="+v+"y\nBRACED_API_TOKEN=${"+rep("eFgH", 3)+"}\n")
	environ := []string{"EXPORTED_ADMIN_SECRET=" + v + "z"}
	log := strings.Join([]string{"echo " + v + "x", "echo " + v + "y", "echo " + v + "z", "echo ${" + rep("eFgH", 3) + "}"}, "\n") + "\n"
	files := collectFiles(t, Options{ProjectDir: dir, Environ: func() []string { return environ }}, logsSource(map[string]string{"app": log}))
	assertAbsent(t, files, v+"x", v+"y", v+"z")
	// a compose-file reference is not a literal
	if _, ok := NewSecretValue("ADMIN_SECRET", "${OTHER_SECRET}"); ok {
		t.Error("an exported ${REF} became a literal through NewSecretValue")
	}
	in := newInputSet(t.TempDir())
	in.learnCompose("ADMIN_SECRET", "${OTHER_SECRET}")
	for _, sv := range in.values {
		for _, s := range sv.Variants {
			if strings.HasPrefix(s, "${OTHER") {
				t.Errorf("compose reference learned as a literal: %q", s)
			}
		}
	}
}

// Item 5: JSON \uXXXX spellings of non-ASCII values, both hex cases and
// surrogate pairs (Python json.dumps, .NET, JavaScript).
func TestCodexJSONUnicodeEscapes(t *testing.T) {
	raw := "pw-éü世-\U0001F600-" + rep("Zq", 4)
	esc := func(upper bool) string {
		var sb strings.Builder
		for _, r := range raw {
			switch {
			case r < 0x80:
				sb.WriteRune(r)
			default:
				for _, u := range utf16.Encode([]rune{r}) {
					if upper {
						fmt.Fprintf(&sb, `\u%04X`, u)
					} else {
						fmt.Fprintf(&sb, `\u%04x`, u)
					}
				}
			}
		}
		return sb.String()
	}
	for _, upper := range []bool{false, true} {
		line := `{"level":"info","msg":"login ` + esc(upper) + `"}`
		if out := NewScrubber(ExpandSecretValues("API_KEY", raw)).Scrub(line); strings.Contains(out, `\u`) {
			t.Errorf("upper=%v: escaped spelling survived: %q", upper, out)
		}
	}
}

// Item 6: Azure SAS (and presigned URL) signatures.
func TestCodexSASSignature(t *testing.T) {
	sig := rep("Ab%2B", 8) + "%3D"
	for _, u := range []string{
		"GET https://acct.blob.core.windows.net/c/b.txt?sv=2022-11-02&sp=r&sr=b&sig=" + sig + "&se=2026-10-12T00:00:00Z",
		"https://acct.blob.core.windows.net/c/b.txt?sv=2022-11-02&sig=" + sig,
		"&lt;Url&gt;https://a.blob.core.windows.net/c?sp=r&amp;sig=" + sig + "&amp;se=2026-10-12&lt;/Url&gt;",
		"https://bucket.s3.example/obj?X-Amz-Signature=" + sig + "&X-Amz-Expires=60",
	} {
		if out := NewScrubber(nil).Scrub(u); strings.Contains(out, sig) {
			t.Errorf("signature survived in %q: %q", u, out)
		}
	}
	keep := "https://acct.blob.core.windows.net/c/b.txt?sv=2022-11-02&sp=r&sr=b&se=2026-10-12"
	if out := NewScrubber(nil).Scrub(keep); out != keep {
		t.Errorf("non-secret SAS fields were rewritten: %q", out)
	}
}

// Item 7: a signed JWT whose payload is short (claims {}) with no Bearer
// prefix and no secret-named assignment.
func TestCodexShortPayloadJWT(t *testing.T) {
	b64 := base64.RawURLEncoding.EncodeToString
	sig := b64([]byte(rep("sig!", 8)))
	for _, claims := range []string{"{}", `{"a":1}`, `{"sub":"` + rep("u", 30) + `"}`} {
		tok := b64([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + b64([]byte(claims)) + "." + sig
		for _, line := range []string{"session " + tok + " ok", `{"msg":"` + tok + `"}`, "x=" + tok} {
			if out := NewScrubber(nil).Scrub(line); strings.Contains(out, sig) || strings.Contains(out, tok) {
				t.Errorf("claims %s: JWT survived in %q: %q", claims, line, out)
			}
		}
	}
	if out := NewScrubber(nil).Scrub("see eyJ.notes.here"); out != "see eyJ.notes.here" {
		t.Errorf("prose rewritten: %q", out)
	}
}
