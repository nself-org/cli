package commands

// Live JSON contract (debt D-0243).
//
// Purpose: run a representative set of read-only envelope commands with
// --json in v1.5 mode and validate the real stdout through
// contractValidateBound (envelope schema, then the data schema the registry
// binds to the document's own `command`). Committed fixtures alone cannot
// catch a live-output drift; this does.
//
// Inputs: contractLiveCommands. Each command runs in a fresh child of this
// test binary that sets os.Args and calls Execute(), the same dispatch main()
// uses (argv rewrite, v1.5 tree preparation, invocation decorator), so no
// separate binary is built and no global state leaks between commands.
//
// Constraints: temp HOME and project; a failing `docker` stub first on PATH;
// proxy variables point at a closed local port and each child has a deadline,
// so a network attempt fails fast instead of hanging. Fixture values are
// obviously fake. Nothing writes outside t.TempDir.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// contractLiveChildEnv carries the child's argv (unit-separator joined).
const contractLiveChildEnv = "NSELF_JSON_CONTRACT_CHILD_ARGS"

// contractLiveSecret is the fake password in the fixture .env; no command in
// the set may print it.
const contractLiveSecret = "fake-contract-pass-0000"

// contractLiveCommands is the representative read-only set: every one runs
// offline in the fixture project (no Docker, no network). `command` is the
// envelope `command` the run must report; `exit` is the expected exit code, a
// documented one from the command's registry row (status: 10, services
// unhealthy because Docker is a failing stub).
var contractLiveCommands = []struct {
	command string
	args    []string
	exit    int
}{
	{"help", []string{"help"}, 0},
	{"help topics", []string{"help", "topics"}, 0},
	{"version", []string{"version"}, 0},
	{"status", []string{"status"}, 10},
	{"status urls", []string{"status", "urls"}, 0},
	{"config show", []string{"config", "show"}, 0},
	{"config list", []string{"config", "list"}, 0},
	{"config get", []string{"config", "get", "PROJECT_NAME"}, 0},
	{"config explain", []string{"config", "explain", "PROJECT_NAME"}, 0},
	{"config validate", []string{"config", "validate"}, 0},
	{"config env list", []string{"config", "env", "list"}, 0},
	{"config env target list", []string{"config", "env", "target", "list"}, 0},
	{"config features list", []string{"config", "features", "list"}, 0},
	{"config telemetry status", []string{"config", "telemetry", "status"}, 0},
	{"config trust ssl status", []string{"config", "trust", "ssl", "status"}, 0},
	{"backup list", []string{"backup", "list"}, 0},
	{"backup status", []string{"backup", "status"}, 0},
	{"backup config", []string{"backup", "config"}, 0},
	{"backup pitr status", []string{"backup", "pitr", "status"}, 0},
	{"db seed list", []string{"db", "seed", "list"}, 0},
	{"db seed fixtures list", []string{"db", "seed", "fixtures", "list"}, 0},
	{"db restore-drill-list", []string{"db", "restore-drill-list"}, 0},
	{"deploy targets", []string{"deploy", "targets"}, 0},
}

func TestJSONContractLive(t *testing.T) {
	if packed := os.Getenv(contractLiveChildEnv); packed != "" {
		contractLiveChild(packed)
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("the docker stub is a POSIX shell script")
	}
	bindings := contractBindings(t)
	for _, c := range contractLiveCommands {
		c := c
		t.Run(c.command, func(t *testing.T) {
			if _, ok := bindings[c.command]; !ok {
				t.Fatalf("%q is not a bound envelope command", c.command)
			}
			stdout, stderr, err := contractLiveRun(t, c.args)
			var exit *exec.ExitError
			code := 0
			if errors.As(err, &exit) {
				code = exit.ExitCode()
			} else if err != nil {
				t.Fatalf("nself %s --json: %v\nstderr:\n%s", strings.Join(c.args, " "), err, stderr)
			}
			if code != c.exit {
				t.Fatalf("nself %s --json: exit %d, want %d\nstderr:\n%s\nstdout:\n%s", strings.Join(c.args, " "), code, c.exit, stderr, stdout)
			}
			doc := singleDoc(t, stdout)
			if doc["command"] != c.command {
				t.Fatalf("envelope command = %v, want %q", doc["command"], c.command)
			}
			if err := contractValidateBound(t, bindings, []byte(stdout)); err != nil {
				t.Fatalf("live output: %v\n%s", err, stdout)
			}
			if strings.Contains(stdout, contractLiveSecret) {
				t.Fatalf("stdout prints the fixture secret:\n%s", stdout)
			}
		})
	}
}

// contractLiveChild is the child side: run argv through Execute() as main()
// does and exit with the command's code. It never returns.
func contractLiveChild(packed string) {
	os.Args = append([]string{"nself"}, strings.Split(packed, "\x1f")...)
	code := 0
	if err := Execute(); err != nil {
		if code = errs.ExitCodeFor(err); code != 0 {
			_, _ = fmt.Fprintf(os.Stderr, "contract child: %v\n", err)
		}
	}
	os.Exit(code)
}

// contractLiveRun runs `nself <args> --json` in a child of this test binary
// inside a fresh fixture project and returns its stdout and stderr.
func contractLiveRun(t *testing.T, args []string) (string, string, error) {
	t.Helper()
	base := t.TempDir()
	proj, home, stub := filepath.Join(base, "proj"), filepath.Join(base, "home"), filepath.Join(base, "stub")
	for _, d := range []string{proj, home, stub} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := "PROJECT_NAME=contract\nENV=dev\nBASE_DOMAIN=local.nself.org\nPOSTGRES_PASSWORD=" + contractLiveSecret + "\n"
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"Cannot connect to the Docker daemon\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stub, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var childEnv []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch {
		case strings.HasPrefix(key, "NSELF_"), strings.HasPrefix(key, "XDG_"), strings.HasSuffix(strings.ToUpper(key), "_PROXY"),
			key == "HOME", key == "USERPROFILE", key == "PATH", key == "DOCKER_HOST":
			continue
		}
		childEnv = append(childEnv, kv)
	}
	closed := "http://127.0.0.1:9"
	childEnv = append(childEnv,
		contractLiveChildEnv+"="+strings.Join(append(append([]string(nil), args...), "--json"), "\x1f"),
		"NSELF_V15=1", "AI_AUTO_INSTALL=false",
		"HOME="+home, "USERPROFILE="+home,
		"PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DOCKER_HOST=unix://"+filepath.Join(base, "no-docker.sock"),
		"HTTP_PROXY="+closed, "HTTPS_PROXY="+closed, "http_proxy="+closed, "https_proxy="+closed,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestJSONContractLive$")
	cmd.Dir, cmd.Env = proj, childEnv
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("timed out (network or docker wait?): %w", err)
	}
	return o.String(), e.String(), err
}
