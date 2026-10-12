package supabase

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/importer"
)

func inputWith(values map[string]string, env map[string]string) importer.Input {
	return importer.Input{Values: values, Env: func(k string) string { return env[k] }}
}

func TestAPIForBuildsDefaultURLFromProjectRef(t *testing.T) {
	a, err := apiFor(inputWith(map[string]string{"project-ref": "abcdefghijkl", "service-key-env": "K"}, map[string]string{"K": fixtureKey}))
	if err != nil || a.base.String() != "https://abcdefghijkl.supabase.co" || a.key != fixtureKey {
		t.Fatalf("%v %v", a, err)
	}
}

func TestAPIForRules(t *testing.T) {
	env := map[string]string{"K": fixtureKey}
	if a, err := apiFor(inputWith(map[string]string{}, env)); a != nil || err != nil {
		t.Fatal("no URL means no storage client")
	}
	if _, err := apiFor(inputWith(map[string]string{"api-url": "http://127.0.0.1:1"}, env)); err == nil || !strings.Contains(err.Error(), "environment variable") {
		t.Fatalf("a URL without a key variable must be refused: %v", err)
	}
	if _, err := apiFor(inputWith(map[string]string{"project-ref": "bad ref!", "service-key-env": "K"}, env)); err == nil {
		t.Fatal("invalid project ref accepted")
	}
	if _, err := apiFor(inputWith(map[string]string{"api-url": "http://evil.example", "service-key-env": "K"}, env)); err == nil {
		t.Fatal("plain http to a remote host accepted")
	}
}

func TestSourceContract(t *testing.T) {
	s := Source{}
	if s.Name() != "supabase" {
		t.Fatal("name")
	}
	env := map[string]bool{}
	for _, f := range s.Flags() {
		env[f.Name] = f.EnvVarName
	}
	if !env["db-url-env"] || !env["service-key-env"] || env["api-url"] || env["project-ref"] {
		t.Fatalf("secret flags must be env-name flags only: %v", env)
	}
	found := false
	for _, r := range importer.Sources() {
		found = found || r.Name() == "supabase"
	}
	if !found {
		t.Fatal("not registered")
	}
}
