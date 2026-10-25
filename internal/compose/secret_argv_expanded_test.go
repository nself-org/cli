package compose_test

// secret_argv_expanded_test.go — D-0587 guard: the command a shell EXPANDS
// inside the cache container carries no secret on its argv. secret_argv_test
// checks the compose text; a `$$REDISCLI_AUTH` there is fine, yet `sh -c` turns
// it into `--requirepass <value>` and host `ps` or /proc/<pid>/cmdline shows
// it. This test runs the generated command with a stub server and reads the
// argv the server really receives.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
)

// awkwardPassword holds every character the config line has to escape or the
// shell could mangle.
const awkwardPassword = "p\"a\\ss w$o`rd'x;#9"

// runCacheCommand runs the sh -c script with a stub engine binary on PATH and
// returns what the stub saw: its argv (one argument per line) and its stdin.
func runCacheCommand(t *testing.T, server, script string) (argv, stdin string) {
	t.Helper()
	dir := t.TempDir()
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + dir + "/argv\"\ncat > \"" + dir + "/stdin\"\n"
	if err := os.WriteFile(filepath.Join(dir, server), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	// compose turns $$ into $ before the container shell sees the script.
	cmd := exec.Command("sh", "-c", strings.ReplaceAll(script, "$$", "$"))
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "REDISCLI_AUTH="+awkwardPassword)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %q: %v\n%s", script, err, out)
	}
	a, err := os.ReadFile(filepath.Join(dir, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := os.ReadFile(filepath.Join(dir, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	return string(a), string(s)
}

// TestExpandedCacheCommandNoSecretOnArgv: the expanded server argv carries no
// part of the password and the server still receives it as a config line.
func TestExpandedCacheCommandNoSecretOnArgv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX sh")
	}
	compattest.Set(t, false)
	for _, cache := range []string{"redis", "valkey"} {
		cfg := optionalSecretsConfig("meilisearch", setSecrets)
		cfg.Redis.Engine = cache
		cfg.Redis.Password = awkwardPassword
		out, err := compose.NewGenerator(cfg).Generate()
		if err != nil {
			t.Fatalf("%s: Generate() error: %v", cache, err)
		}
		cmd := argvStrings(t, build.TemplateSecrets(out, build.SecretEnvMap(cfg)))["redis command"]
		if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" {
			t.Fatalf("%s: redis command = %q, want sh -c <script>", cache, cmd)
		}
		argv, stdin := runCacheCommand(t, cache+"-server", cmd[2])
		if strings.Contains(argv, "pass") || strings.Contains(argv, "rd'x") || strings.Contains(argv, "requirepass") {
			t.Errorf("%s: server argv carries the password:\n%s", cache, argv)
		}
		for _, want := range []string{"--appendonly", "--protected-mode", "--maxclients"} {
			if !strings.Contains(argv, want) {
				t.Errorf("%s: server argv lost %s:\n%s", cache, want, argv)
			}
		}
		if want := "requirepass \"p\\\"a\\\\ss w$o`rd'x;#9\"\n"; stdin != want {
			t.Errorf("%s: server stdin = %q, want the escaped config line %q", cache, stdin, want)
		}
	}
}

// TestExpandedCacheCommandControl proves the check can fail: the pre-D-0587
// command puts the password on the server's argv.
func TestExpandedCacheCommandControl(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX sh")
	}
	argv, _ := runCacheCommand(t, "redis-server", `exec redis-server --appendonly yes --requirepass "$$REDISCLI_AUTH" --maxclients 50`)
	if !strings.Contains(argv, awkwardPassword) {
		t.Fatalf("control: the legacy command should expose the password on argv, got:\n%s", argv)
	}
}
