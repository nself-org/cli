package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
)

// d0430Scalars are synthetic credential-shaped scalars written into nself.yaml
// where build's validator echoes a value: a string where a map or list
// belongs, and non-string scalars where a string belongs.
var d0430Scalars = []string{"fake-scalar-d0430-0000", "fake-list-d0430-0000", "8675309123", "fake-true",
	"fake-pem-d0430-0000", "fake-pem-d0430-0001", "fake-tag-d0430-0000"}

// d0430Manifests cover a one-line string, a block scalar (newlines in the
// value) and a custom tag, plus non-string scalars where a string belongs.
var d0430Manifests = []string{
	"plugins: fake-scalar-d0430-0000\nbundles: fake-list-d0430-0000\ncli_min_version: 8675309123\napp: !!bool fake-true\nbundle: !my-tag fake-tag-d0430-0000\n",
	"plugins: |\n  fake-pem-d0430-0000\n  fake-pem-d0430-0001\n",
}

// TestRedactManifestFinding runs build.ValidateManifestFile on a manifest that
// holds synthetic scalars in every echoing shape (D-0430). The raw findings
// must carry the scalars (positive control: the guard tracks build's wording)
// and the redacted findings must not, while file, line and path stay.
func TestRedactManifestFinding(t *testing.T) {
	var findings []build.Finding
	for i, m := range d0430Manifests {
		path := filepath.Join(t.TempDir(), "nself.yaml")
		if err := os.WriteFile(path, []byte(m), 0o600); err != nil {
			t.Fatal(err)
		}
		if fs := build.ValidateManifestFile(path); len(fs) > 0 {
			findings = append(findings, fs...)
		} else {
			t.Fatalf("manifest %d gives no finding", i)
		}
	}
	var raw, redacted strings.Builder
	for _, f := range findings {
		raw.WriteString(f.String() + "\n")
		r := redactManifestFinding(f)
		if r.File != f.File || r.Line != f.Line || r.Path != f.Path || r.Code != f.Code {
			t.Errorf("redaction changed the location: %+v -> %+v", f, r)
		}
		redacted.WriteString(r.String() + "\n")
	}
	for _, v := range d0430Scalars {
		if v != "fake-true" && !strings.Contains(raw.String(), v) {
			t.Fatalf("positive control: build no longer echoes %q, so this guard tests nothing; update redactManifestFinding\n%s", v, raw.String())
		}
	}
	for _, v := range d0430Scalars {
		if strings.Contains(redacted.String(), v) {
			t.Errorf("redacted findings still hold %q:\n%s", v, redacted.String())
		}
	}
	if !strings.Contains(redacted.String(), "(value hidden)") {
		t.Errorf("redacted findings lack the (value hidden) marker:\n%s", redacted.String())
	}
}

// TestConfigValidateHidesManifestScalars: machine `config validate` keeps the
// manifest findings but no scalar from nself.yaml reaches stdout or stderr.
func TestConfigValidateHidesManifestScalars(t *testing.T) {
	for i, m := range d0430Manifests {
		proj := cfgFixtureProject(t)
		if err := os.WriteFile(filepath.Join(proj, "nself.yaml"), []byte(m), 0o600); err != nil {
			t.Fatal(err)
		}
		val := runPilot(t, proj, true, nil, "config", "validate", "--json")
		data := validateEnvelope(t, val.stdout, "config validate")
		if findings, _ := data["manifest_findings"].([]any); len(findings) == 0 {
			t.Fatalf("manifest %d: validate data lists no manifest finding: %v", i, data)
		}
		for _, v := range d0430Scalars {
			if strings.Contains(val.stdout+val.stderr, v) {
				t.Errorf("manifest %d: config validate --json echoes the manifest scalar %q:\n%s\n%s", i, v, val.stdout, val.stderr)
			}
		}
	}
}

// TestConfigGetMasksEncodedURLParams: machine `config get` and `config show`
// mask a credential carried in a percent-encoded query name or a URL fragment
// without --reveal (D-0430). Values are synthetic.
func TestConfigGetMasksEncodedURLParams(t *testing.T) {
	proj := cfgFixtureProject(t)
	extra := "\nCALLBACK_URL=https://example.test/?access%5Ftoken=fake-enc-d0430-0000" +
		"\nREDIRECT_URL=https://example.test/cb#state=s1&access_token=fake-frag-d0430-0000\n"
	for _, f := range []string{".env", ".env.dev"} {
		fh, err := os.OpenFile(filepath.Join(proj, f), os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fh.WriteString(extra); err != nil {
			t.Fatal(err)
		}
		_ = fh.Close()
	}
	needles := []string{"fake-enc-d0430-0000", "fake-frag-d0430-0000"}
	for _, argv := range [][]string{
		{"config", "get", "CALLBACK_URL", "--json"},
		{"config", "get", "REDIRECT_URL", "--json"},
		{"config", "show", "--json"},
	} {
		res := runPilot(t, proj, true, nil, argv...)
		if !strings.Contains(res.stdout, "****") {
			t.Errorf("%s: no mask in output (exit %d):\n%s", strings.Join(argv, " "), res.code, res.stdout)
		}
		for _, v := range needles {
			if strings.Contains(res.stdout+res.stderr, v) {
				t.Errorf("%s: output holds %q:\n%s", strings.Join(argv, " "), v, res.stdout)
			}
		}
	}
	// Positive control: --reveal prints the value, so the needle check can see it.
	ctl := runPilot(t, proj, true, nil, "config", "export", "--reveal", "--json")
	if !strings.Contains(ctl.stdout, "fake-frag-d0430-0000") {
		t.Fatalf("positive control: export --reveal exit %d lacks the fixture value:\n%s", ctl.code, ctl.stdout)
	}
}
