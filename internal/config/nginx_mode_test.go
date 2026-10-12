package config

import (
	"strings"
	"testing"
)

func TestNginxMode(t *testing.T) {
	cases := []struct {
		name         string
		mode, front  string
		owner        bool
		problemMatch string
	}{
		{"unset", "", "", false, ""},
		{"shared owner", "shared", "", true, ""},
		{"fronted only", "", "owner-stack", false, ""},
		{"unknown value", "bogus", "", false, "not a known mode"},
		{"wrong case", "Shared", "", false, "not a known mode"},
		{"shared and fronted", "shared", "owner-stack", false, "cannot be combined"},
	}
	for _, c := range cases {
		cfg := &Config{}
		cfg.Nginx.Mode, cfg.Nginx.FrontedBy = c.mode, c.front
		if got := cfg.NginxSharedOwner(); got != c.owner {
			t.Errorf("%s: NginxSharedOwner = %v, want %v", c.name, got, c.owner)
		}
		got := cfg.NginxModeProblem()
		if (c.problemMatch == "") != (got == "") || !strings.Contains(got, c.problemMatch) {
			t.Errorf("%s: NginxModeProblem = %q, want it to contain %q", c.name, got, c.problemMatch)
		}
	}
}

func TestNginxModeParsedFromEnv(t *testing.T) {
	t.Setenv("NGINX_MODE", " shared ")
	cfg := &Config{}
	// loader_parse_env_core.go fills Nginx from the environment.
	parseEnvCore(cfg)
	if cfg.Nginx.Mode != NginxModeShared {
		t.Fatalf("NGINX_MODE not parsed: %q", cfg.Nginx.Mode)
	}
}
