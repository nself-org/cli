package remote

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCommand_RefusesUnknownTool(t *testing.T) {
	n := countExecs(t)
	for _, tool := range []string{"sh", "bash", "ssh-add", "/usr/bin/ssh", "ssh ", ""} {
		if _, err := Command(context.Background(), tool, "-c", "id"); err == nil {
			t.Errorf("Command(%q) = nil error", tool)
		}
	}
	if *n != 0 {
		t.Fatalf("hook called %d times for refused tools", *n)
	}
}

func TestCommand_RefusesNUL(t *testing.T) {
	n := countExecs(t)
	if _, err := Command(context.Background(), "ssh", "a\x00b"); err == nil || *n != 0 {
		t.Fatalf("NUL argument not refused before exec (err=%v, execs=%d)", err, *n)
	}
}

func TestEnvAllowlist_OnlyAllowedNames(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "x")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "y")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("LANG", "en_US.UTF-8")
	allowed := map[string]bool{"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SSH_AUTH_SOCK": true, "LANG": true}
	var sawLang bool
	for _, kv := range EnvAllowlist() {
		k, v, _ := strings.Cut(kv, "=")
		if !allowed[k] {
			t.Errorf("EnvAllowlist leaked %s", k)
		}
		if k == "LANG" {
			sawLang = true
			if v != "C" {
				t.Errorf("LANG = %q, want C", v)
			}
		}
	}
	if !sawLang {
		t.Error("LANG=C missing")
	}
}

func TestEveryExecRunsWithAllowlistedEnv(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "leak-me")
	t.Setenv("HOME", t.TempDir())
	log := stubTools(t, map[string]string{"ssh": "exit 0", "scp": "exit 0", "rsync": "exit 0", "ssh-keyscan": "exit 0"})
	ctx := context.Background()
	tg := Target{Dest: "deploy@203.0.113.7", Options: CIOptions("n1", "/k/known_hosts", Version{9, 6})}
	if _, err := Run(ctx, tg, "true"); err != nil {
		t.Fatal(err)
	}
	if err := CopyTo(ctx, tg, "/tmp/a", "/opt/a"); err != nil {
		t.Fatal(err)
	}
	if err := Rsync(ctx, tg, []string{"-az"}, "/tmp/a", "/opt/a"); err != nil {
		t.Fatal(err)
	}
	_, _ = ScanHostKeys(ctx, "203.0.113.7", 22) // no output: error expected, exec still happened
	calls := readCalls(t, log)
	if len(calls) != 4 {
		t.Fatalf("got %d execs, want 4", len(calls))
	}
	allowed := map[string]bool{"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SSH_AUTH_SOCK": true, "LANG": true, "PWD": true, "SHLVL": true, "_": true, "OLDPWD": true}
	for _, c := range calls {
		for _, kv := range c.env {
			k, _, _ := strings.Cut(kv, "=")
			if !allowed[k] {
				t.Errorf("%s ran with non-allowlisted env %s", c.tool, k)
			}
			if k == "SECRET_TOKEN" {
				t.Errorf("%s inherited SECRET_TOKEN", c.tool)
			}
		}
	}
	_ = os.Getenv
}
