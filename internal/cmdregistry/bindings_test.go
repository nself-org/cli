package cmdregistry

import (
	"strings"
	"testing"
)

func TestRegistryFlagEnv(t *testing.T) {
	moves := []PathMove{{From: "env", To: "config env"}, {From: "health", To: "status health"}}
	flagEnv := map[string]map[string]string{
		"status": {"watch": "WATCH_KEY"},
		"doctor": {"fix": "FIX_KEY"},
		".":      {"config": "CONFIG_KEY"},
	}
	build := func(t *testing.T) *Registry {
		t.Helper()
		reg, err := Build(fixtureRoot(false), fixtureCanon(t), map[string]any{}, BuildOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return reg
	}
	envOf := func(reg *Registry, path, flag string) *string {
		t.Helper()
		var flags []Flag
		if path == "." {
			flags = reg.Root.Flags
		} else {
			c, ok := reg.Lookup(path)
			if !ok {
				t.Fatalf("no command %s", path)
			}
			flags = c.Flags
		}
		for _, f := range flags {
			if f.Name == flag {
				return f.Env
			}
		}
		t.Fatalf("no flag %s on %s", flag, path)
		return nil
	}

	t.Run("v15 fills env on bound flags only", func(t *testing.T) {
		reg := build(t)
		if err := ApplyFlagEnv(reg, flagEnv, moves, true); err != nil {
			t.Fatal(err)
		}
		if got := envOf(reg, "status", "watch"); got == nil || *got != "WATCH_KEY" {
			t.Errorf("status --watch env = %v", got)
		}
		if got := envOf(reg, ".", "config"); got == nil || *got != "CONFIG_KEY" {
			t.Errorf("root --config env = %v", got)
		}
		if got := envOf(reg, "status", "json"); got != nil {
			t.Errorf("unbound flag has env %q", *got)
		}
	})

	t.Run("v14 translates through moves", func(t *testing.T) {
		reg := build(t)
		fe := map[string]map[string]string{"status health": {"x": "K"}}
		err := ApplyFlagEnv(reg, fe, moves, false)
		if err == nil || !strings.Contains(err.Error(), `command "health" does not resolve`) {
			t.Fatalf("v14 path not translated: %v", err)
		}
		if err := ApplyFlagEnv(build(t), fe, moves, true); err == nil || !strings.Contains(err.Error(), `command "status health" does not resolve`) {
			t.Fatalf("v15 path used verbatim: %v", err)
		}
	})

	t.Run("missing flag and missing command are errors", func(t *testing.T) {
		err := ApplyFlagEnv(build(t), map[string]map[string]string{"status": {"nope": "K"}, "ghost": {"a": "K"}}, nil, true)
		if err == nil || !strings.Contains(err.Error(), "has no flag --nope") || !strings.Contains(err.Error(), `command "ghost" does not resolve`) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestV14Path(t *testing.T) {
	moves := []PathMove{{"env", "config env"}, {"x", "config"}, {"dev", "start dev"}}
	tests := map[string]string{
		"config env":         "env",
		"config env explain": "env explain",
		"config show":        "x show",
		"start dev":          "dev",
		"init":               "init",
		"configure":          "configure",
	}
	for in, want := range tests {
		if got := V14Path(in, moves); got != want {
			t.Errorf("V14Path(%q) = %q, want %q", in, got, want)
		}
	}
}
