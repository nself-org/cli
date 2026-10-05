package remote

// Purpose: shared test helpers: stub ssh/scp/rsync/ssh-keyscan scripts on PATH
// and an exec-count hook. No test here connects to a host.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubTools writes one shell script per tool into a temp dir placed first on
// PATH. Each script appends "TOOL <name>", one "ARG <x>" line per argument and
// the sorted environment ("ENV k=v") to the returned log file, then runs body.
func stubTools(t *testing.T, bodies map[string]string) (logFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// The stubs are POSIX shell scripts; Windows would resolve the real
		// ssh.exe instead. Never run these tests against a real client.
		t.Skip("stub ssh/scp/rsync are POSIX shell scripts")
	}
	dir := t.TempDir()
	logFile = filepath.Join(dir, "calls.log")
	for tool, body := range bodies {
		script := "#!/bin/sh\n" +
			"{ echo \"TOOL " + tool + "\"; for a in \"$@\"; do printf 'ARG %s\\n' \"$a\"; done;" +
			" env | sort | sed 's/^/ENV /'; echo END; } >> \"" + logFile + "\"\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(dir, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile
}

type call struct {
	tool string
	args []string
	env  []string
}

func readCalls(t *testing.T, logFile string) []call {
	t.Helper()
	raw, err := os.ReadFile(logFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []call
	var cur call
	for _, l := range strings.Split(string(raw), "\n") {
		switch {
		case strings.HasPrefix(l, "TOOL "):
			cur = call{tool: strings.TrimPrefix(l, "TOOL ")}
		case strings.HasPrefix(l, "ARG "):
			cur.args = append(cur.args, strings.TrimPrefix(l, "ARG "))
		case strings.HasPrefix(l, "ENV "):
			cur.env = append(cur.env, strings.TrimPrefix(l, "ENV "))
		case l == "END":
			out = append(out, cur)
		}
	}
	return out
}

// countExecs replaces the commandContext hook with one that counts starts and
// runs `true`. It returns the counter; callers assert 0 for refused input.
func countExecs(t *testing.T) *int {
	t.Helper()
	n := 0
	orig := commandContext
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		n++
		return exec.CommandContext(ctx, "true")
	}
	t.Cleanup(func() { commandContext = orig })
	return &n
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

func pathEnv() string { return os.Getenv("PATH") }
