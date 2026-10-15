package build

// Purpose: D-0640 regressions for decoded scalar values in every YAML shape.
// Inputs: synthetic compose text and secrets containing YAML escapes.
// Outputs: valid YAML whose references resolve to the original values.
// Constraints: mapping keys, comments and unrelated bytes stay unchanged.
// SPORT: cli/internal/build — secret templating (S-secret-env-templating).

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTemplateSecrets_ScalarSequences(t *testing.T) {
	for _, secret := range []string{`fake"pw\$abc`, "fake'pw$abc", "fake\tpw$abc", "fake\npw$abc", "fake\x01pw$abc"} {
		for _, style := range []yaml.Style{0, yaml.FlowStyle} {
			doc := yaml.Node{Kind: yaml.SequenceNode, Style: style, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Tag: "!!str", Value: "prefix " + secret + " suffix"},
			}}
			in, err := yaml.Marshal(&doc)
			if err != nil {
				t.Fatal(err)
			}
			assertScalarTemplating(t, string(in), secret)
		}
	}
}

func TestTemplateSecrets_BlockScalars(t *testing.T) {
	const secret = `fake"pw\$abc`
	for _, header := range []string{"|", "|-", "|+", ">", ">-", ">+", "|2-", "!!str |", "&credential |"} {
		t.Run(header, func(t *testing.T) {
			in := "services:\n  app:\n    command: " + header + " # keep header\n      prefix " + secret + " suffix\n\n    image: alpine # keep image\n"
			out := assertScalarTemplating(t, in, secret)
			for _, keep := range []string{"# keep header", "\n    image: alpine # keep image\n"} {
				if !strings.Contains(out, keep) {
					t.Errorf("lost untouched bytes %q:\n%s", keep, out)
				}
			}
			if header == "&credential |" && !strings.Contains(out, "&credential") {
				t.Errorf("lost scalar anchor:\n%s", out)
			}
		})
	}
}

func TestTemplateSecrets_ScalarBytesAndKeys(t *testing.T) {
	const secret = `fake"pw\$abc`
	in := "# keep fake\"pw\\$abc comment\nservices: {app: {command: ['é: fake\"pw\\$abc', unchanged], image: alpine}} # tail\n" +
		"'fake\"pw\\$abc': untouched\n"
	want := strings.Replace(in, "'é: "+secret+"'", `"é: ${MINIO_ROOT_PASSWORD}"`, 1)
	out := assertScalarTemplating(t, in, secret)
	if out != want {
		t.Errorf("changed unrelated bytes:\ngot:  %s\nwant: %s", out, want)
	}
}

func TestTemplateSecrets_MultilineAndAliases(t *testing.T) {
	const secret = `fake"pw\$abc`
	for _, in := range []string{
		"command: prefix\n  " + secret + " suffix\nimage: alpine\n",
		"command: 'prefix\n  " + secret + " suffix'\nimage: alpine\n",
		"command: &credential '" + secret + "'\nother: *credential\n",
		"command: !!str '" + secret + "'\n",
		"command: ['" + secret + "', '${MINIO_ROOT_PASSWORD}']\n",
	} {
		assertScalarTemplating(t, in, secret)
	}
}

func assertScalarTemplating(t *testing.T, in, secret string) string {
	t.Helper()
	secrets := map[string]string{"MINIO_ROOT_PASSWORD": secret}
	out := string(TemplateSecrets([]byte(in), secrets))
	if !strings.Contains(out, "${MINIO_ROOT_PASSWORD}") {
		t.Errorf("secret was not templated:\n%s", out)
	}
	var before, after any
	if err := yaml.Unmarshal([]byte(in), &before); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(out), &after); err != nil {
		t.Fatalf("invalid rewritten YAML: %v\n%s", err, out)
	}
	if got := resolveScalarValues(after, secrets); !reflect.DeepEqual(got, resolveScalarValues(before, secrets)) {
		t.Errorf("resolved values changed:\ngot: %#v\nwant: %#v\n%s", got, before, out)
	}
	return out
}

func resolveScalarValues(v any, secrets map[string]string) any {
	switch v := v.(type) {
	case string:
		return resolveSecrets(v, secrets)
	case []any:
		for i := range v {
			v[i] = resolveScalarValues(v[i], secrets)
		}
	case map[string]any:
		for k := range v {
			v[k] = resolveScalarValues(v[k], secrets)
		}
	case map[any]any:
		for k := range v {
			v[k] = resolveScalarValues(v[k], secrets)
		}
	}
	return v
}
