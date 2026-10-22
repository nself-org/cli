package build

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTemplateOtherLiteralScalars(t *testing.T) {
	for _, secret := range []string{"fake'quoted$password", `fake\slash$password`, "fake\tcontrol$password", "fake\nmultiline$password"} {
		for _, style := range []yaml.Style{0, yaml.DoubleQuotedStyle, yaml.LiteralStyle, yaml.FoldedStyle} {
			value := "prefix " + secret + " suffix"
			n := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Tag: "!!str", Value: secret},
				{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: style}}},
			}}
			in, err := yaml.Marshal(n)
			if err != nil {
				t.Fatal(err)
			}
			out := templateOtherLiterals(string(in), []string{"POSTGRES_PASSWORD"}, map[string]string{"POSTGRES_PASSWORD": secret})
			var got yaml.Node
			if err := yaml.Unmarshal([]byte(out), &got); err != nil {
				t.Fatal(err)
			}
			mapping := got.Content[0]
			if mapping.Content[0].Value != secret {
				t.Fatal("mapping key changed")
			}
			scalar := mapping.Content[1].Content[0].Value
			if scalar != "prefix ${POSTGRES_PASSWORD} suffix" {
				t.Error("value was not templated")
			}
			if strings.ReplaceAll(scalar, "${POSTGRES_PASSWORD}", secret) != value {
				t.Error("resolved bytes changed")
			}
			if again := templateOtherLiterals(out, []string{"POSTGRES_PASSWORD"}, map[string]string{"POSTGRES_PASSWORD": secret}); again != out {
				t.Error("rewrite is not byte stable")
			}
		}
	}
}

func TestTemplateNumericValueScalar(t *testing.T) {
	const secret = "1234567890123456"
	out := templateOtherLiterals("values: ["+secret+"]\n", []string{"POSTGRES_PASSWORD"}, map[string]string{"POSTGRES_PASSWORD": secret})
	var got map[string][]string
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got["values"]) != 1 || got["values"][0] != "${POSTGRES_PASSWORD}" {
		t.Error("numeric value scalar retains the secret")
	}
}
