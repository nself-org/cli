package build

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTemplateOtherLiteralsValueScalars(t *testing.T) {
	for _, secret := range []string{"fixture'quote$name", "fixture\"quote$name", "fixture\\slash$name", "fixture\ncontrol$name", "fixture\tcontrol$name"} {
		input := map[string]any{secret: "key stays", "command": []string{"prefix " + secret + " suffix"}, "block": "prefix\n" + secret + "\nsuffix\n", "reference": "${POSTGRES_PASSWORD}"}
		encoded, err := yaml.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		secrets := map[string]string{"POSTGRES_PASSWORD": secret}
		out := string(TemplateSecrets(encoded, secrets))
		var got map[string]any
		if err := yaml.Unmarshal([]byte(out), &got); err != nil {
			t.Fatal(err)
		}
		if got[secret] != "key stays" {
			t.Fatal("mapping key changed")
		}
		if got["command"].([]any)[0] != "prefix ${POSTGRES_PASSWORD} suffix" {
			t.Fatal("sequence literal remains")
		}
		if got["block"] != "prefix\n${POSTGRES_PASSWORD}\nsuffix\n" {
			t.Fatal("block literal remains or bytes changed")
		}
		if resolveSecrets(got["command"].([]any)[0].(string), secrets) != "prefix "+secret+" suffix" {
			t.Fatal("sequence did not round-trip")
		}
		if resolveSecrets(got["block"].(string), secrets) != "prefix\n"+secret+"\nsuffix\n" {
			t.Fatal("block did not round-trip")
		}
		if got["reference"] != "${POSTGRES_PASSWORD}" {
			t.Fatal("reference changed")
		}
		again := templateOtherLiterals(out, []string{"POSTGRES_PASSWORD"}, map[string]string{"POSTGRES_PASSWORD": secret})
		if out != again {
			t.Fatal("rewrite is not stable")
		}
	}
}
