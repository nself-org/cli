package commands

// config_explain_args_test.go — argument parsing and the ENV selector of
// `nself config explain` (P7-SURF-06 post-merge fixes).
//
// Purpose: prove boolean options take strconv.ParseBool values, root
//          persistent flags are options unless they are the flag asked about,
//          a KEY with a stray flag is E401 usage, and `config explain ENV`
//          names the source that selected the cascade.
// Constraints: parseExplainArgs is pure; the ENV case runs the command body
//          against a temp project directory.

import (
	"os"
	"strings"
	"testing"
)

func TestParseExplainArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		words   []string
		flag    string
		reveal  bool
		json    bool
		globals map[string]string
		code    string // E401 when the arguments are not usable
	}{
		{"key", []string{"BASE_DOMAIN"}, []string{"BASE_DOMAIN"}, "", false, false, nil, ""},
		{"reveal=true", []string{"BASE_DOMAIN", "--reveal=true"}, []string{"BASE_DOMAIN"}, "", true, false, nil, ""},
		{"reveal=false", []string{"BASE_DOMAIN", "--reveal", "--reveal=false"}, []string{"BASE_DOMAIN"}, "", false, false, nil, ""},
		{"json=1", []string{"BASE_DOMAIN", "--json=1"}, []string{"BASE_DOMAIN"}, "", false, true, nil, ""},
		{"json=TRUE", []string{"BASE_DOMAIN", "--json=TRUE"}, []string{"BASE_DOMAIN"}, "", false, true, nil, ""},
		{"json=0", []string{"BASE_DOMAIN", "--json", "--json=0"}, []string{"BASE_DOMAIN"}, "", false, false, nil, ""},
		{"json=maybe", []string{"BASE_DOMAIN", "--json=maybe"}, nil, "", false, false, nil, "E401"},
		{"reveal=yes", []string{"BASE_DOMAIN", "--reveal=yes"}, nil, "", false, false, nil, "E401"},
		{"key with root flag", []string{"BASE_DOMAIN", "--no-deprecation-warnings"}, []string{"BASE_DOMAIN"}, "", false, false, map[string]string{"no-deprecation-warnings": "true"}, ""},
		{"key with no-monorepo", []string{"BASE_DOMAIN", "--no-monorepo=false"}, []string{"BASE_DOMAIN"}, "", false, false, map[string]string{"no-monorepo": "false"}, ""},
		{"root flag value not bool", []string{"BASE_DOMAIN", "--no-monorepo=x"}, nil, "", false, false, nil, "E401"},
		{"command flag plus root flag", []string{"init", "--domain", "--no-monorepo"}, []string{"init"}, "domain", false, false, map[string]string{"no-monorepo": "true"}, ""},
		{"root flag is the target", []string{"build", "--no-monorepo"}, []string{"build"}, "no-monorepo", false, false, map[string]string{}, ""},
		{"two root flags and no target", []string{"build", "--no-monorepo", "--no-deprecation-warnings"}, nil, "", false, false, nil, "E401"},
		{"two target flags", []string{"init", "--domain", "--debug"}, nil, "", false, false, nil, "E401"},
		{"key with a stray flag", []string{"BASE_DOMAIN", "--domain"}, nil, "", false, false, nil, "E401"},
		{"short option", []string{"BASE_DOMAIN", "-x"}, nil, "", false, false, nil, "E401"},
		{"leading nself", []string{"nself", "init", "--domain"}, []string{"init"}, "domain", false, false, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := parseExplainArgs(tt.args)
			if tt.code != "" {
				if err == nil {
					t.Fatalf("want %s, got %+v", tt.code, in)
				}
				if c := explainCode(t, err); c != tt.code {
					t.Fatalf("code %s, want %s (%v)", c, tt.code, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(in.words, " ") != strings.Join(tt.words, " ") || in.flag != tt.flag ||
				in.reveal != tt.reveal || in.json != tt.json {
				t.Errorf("got words=%v flag=%q reveal=%v json=%v", in.words, in.flag, in.reveal, in.json)
			}
			if len(in.globals) != len(tt.globals) {
				t.Fatalf("globals = %v, want %v", in.globals, tt.globals)
			}
			for k, v := range tt.globals {
				if in.globals[k] != v {
					t.Errorf("globals = %v, want %v", in.globals, tt.globals)
				}
			}
		})
	}
}

func TestConfigExplainValidFlagsRun(t *testing.T) {
	// The forms that used to become the target flag and fail with E434.
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	envExplainProject(t, map[string]string{".env": "BASE_DOMAIN=env.test\n"})
	pilotBuffers(t)
	for _, args := range [][]string{
		{"BASE_DOMAIN", "--reveal=true"},
		{"BASE_DOMAIN", "--json=1"},
		{"BASE_DOMAIN", "--json=TRUE"},
		{"BASE_DOMAIN", "--no-deprecation-warnings"},
		{"BASE_DOMAIN", "--no-monorepo"},
		{"init", "--domain", "--no-deprecation-warnings"},
	} {
		if _, err := explainRun(t, args...); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	// A root flag asked about on its own resolves to its exemption.
	_, err := explainRun(t, "build", "--no-monorepo")
	if err == nil || explainCode(t, err) != "E434" || !strings.Contains(err.Error(), "no code reads it") {
		t.Errorf("build --no-monorepo: want the E434 exemption, got %v", err)
	}
}

func TestConfigExplainEnvSelector(t *testing.T) {
	// Repro from the review: .env says ENV=dev, the process says ENV=prod.
	// The process value selects the .env, .env.prod cascade; .env wins the value.
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	envExplainProject(t, map[string]string{".env": "ENV=dev\n", ".env.prod": "PROJECT_NAME=demo\n"})
	t.Setenv("ENV", "prod")
	out, _ := pilotBuffers(t)

	human, err := explainRun(t, "config", "--env", "--reveal")
	if err != nil {
		t.Fatalf("explain config --env: %v", err)
	}
	for _, want := range []string{"Environment: prod", "cascade selected by: process environment", ".env wins: ENV=dev"} {
		if !strings.Contains(human, want) {
			t.Errorf("human output lacks %q:\n%s", want, human)
		}
	}

	if _, err := explainRun(t, "ENV", "--json"); err != nil {
		t.Fatalf("explain ENV --json: %v", err)
	}
	data := singleDoc(t, out.String())["data"].(map[string]any)
	if data["environment"] != "prod" || data["environment_source"] != "process environment" {
		t.Errorf("environment = %v from %v; want prod from process environment", data["environment"], data["environment_source"])
	}

	// Any other key carries the source in JSON but prints no selector line.
	_ = os.Unsetenv("BASE_DOMAIN")
	other, err := explainRun(t, "BASE_DOMAIN")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(other, "cascade selected by") {
		t.Errorf("selector line printed for BASE_DOMAIN:\n%s", other)
	}
}
