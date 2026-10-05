//go:build !windows

package commands

// Tests for the project operation lock taken by the invocation decorator
// (invocation.go, internal/oplock/oplockcmd). Two-process cases re-run this
// test binary as a helper (TestOpLockHelperProcess) that executes one command
// of the fixture tree in a project directory. No test sleeps: waiting is a
// pipe read or an injected clock.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/oplock"
	"github.com/nself-org/cli/internal/oplock/oplockcmd"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// lockFixture is a tree with one command per lock class.
type lockFixture struct {
	root *cobra.Command
	ran  map[string]int
}

func newLockFixture(t *testing.T) *lockFixture {
	t.Helper()
	output.ResetState()
	t.Cleanup(output.ResetState)
	f := &lockFixture{ran: map[string]int{}}
	root := &cobra.Command{Use: "nself", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("json", false, "json")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	body := func(name string, fn func(*cobra.Command) error) *cobra.Command {
		return &cobra.Command{Use: name, RunE: func(c *cobra.Command, _ []string) error {
			f.ran[name]++
			if fn != nil {
				return fn(c)
			}
			return nil
		}}
	}
	cfg := &cobra.Command{Use: "config"}
	cfg.AddCommand(body("set", nil))
	plug := body("plug", nil) // read, but --apply escalates to write
	plug.Flags().Bool("apply", false, "x")
	start := body("start", func(c *cobra.Command) error {
		opts, err := resolveStartOpts(c) // releases the lock when --watch is given
		if err != nil {
			return err
		}
		_ = opts
		// A concurrent write command in another process, without the token.
		return runHelperAs("config set", oplock.EnvToken+"=")
	})
	start.Flags().Bool("watch", false, "x")
	nest := body("nest", func(*cobra.Command) error { return runHelperAs("build") })
	nested := body("nest-bare", func(*cobra.Command) error {
		return runHelperAs("build", oplock.EnvToken+"=")
	})
	build := body("build", func(*cobra.Command) error {
		switch os.Getenv("OPLOCK_MODE") {
		case "panic":
			panic("boom")
		case "error":
			return errors.New("failed")
		}
		if os.Getenv("OPLOCK_BLOCK") != "1" {
			return nil
		}
		fmt.Println("ready")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return nil
	})
	root.AddCommand(build, cfg, body("status", nil), body("logs", nil), plug, start, nest, nested)

	doc := func(side string) *cmdregistry.Command {
		return &cmdregistry.Command{SideEffect: side, Output: canon.OutputDocument, JSON: canon.JSONEnvelope}
	}
	entries := map[string]*cmdregistry.Command{
		"nself build":      doc("write"),
		"nself config set": doc("write"),
		"nself status":     doc("read"),
		"nself start":      doc("write"),
		"nself nest":       doc("write"),
		"nself nest-bare":  doc("write"),
		"nself logs":       {SideEffect: "write", Output: canon.OutputStream, JSON: canon.JSONNone},
		"nself plug": {SideEffect: "read", Output: canon.OutputDocument, JSON: canon.JSONEnvelope,
			Flags: []cmdregistry.Flag{{Name: "apply", SideEffect: str("write")}}},
	}
	old := jsonEntryFor
	jsonEntryFor = func(c *cobra.Command) (*cmdregistry.Command, error) {
		if e, ok := entries[c.CommandPath()]; ok {
			return e, nil
		}
		return nil, errors.New("no entry for " + c.CommandPath())
	}
	t.Cleanup(func() { jsonEntryFor = old })
	installInvocationDecorator(root)
	f.root = root
	return f
}

func (f *lockFixture) run(args ...string) error {
	f.root.SetArgs(args)
	return f.root.Execute()
}

// project makes a project directory (an .env marker) and chdirs into it.
func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("ENV=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func lockFileExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".nself", "op.lock"))
	return err == nil
}

// TestOpLockHelperProcess runs `$OPLOCK_ARGS` of the fixture tree and prints
// "result <E-code|ok>". It does nothing in a normal test run.
func TestOpLockHelperProcess(t *testing.T) {
	args := os.Getenv("OPLOCK_ARGS")
	if args == "" {
		t.Skip("helper process only")
	}
	err := newLockFixture(t).run(strings.Fields(args)...)
	res := "ok"
	if err != nil {
		res = "error"
		if d := errs.Describe(err); d != nil {
			res = d.Code
		}
	}
	fmt.Println("result", res)
	os.Exit(0)
}

// helperCmd builds the helper command for args in the current directory.
func helperCmd(args string, extraEnv ...string) *exec.Cmd {
	c := exec.Command(os.Args[0], "-test.run=^TestOpLockHelperProcess$")
	c.Env = append(append(os.Environ(), "OPLOCK_ARGS="+args), extraEnv...)
	return c
}

// runHelperAs runs the helper to completion and returns an error unless it
// reported ok. Its environment is this process's (token included) plus extra.
func runHelperAs(args string, extraEnv ...string) error {
	out, err := helperCmd(args, extraEnv...).Output()
	if err != nil {
		return fmt.Errorf("helper %q: %w", args, err)
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "result ") {
			if r := strings.TrimPrefix(l, "result "); r != "ok" {
				return errs.New(r, "helper reported "+r)
			}
			return nil
		}
	}
	return fmt.Errorf("helper %q printed no result: %s", args, out)
}

// startBuildHolder runs `build` in a helper that blocks while holding the lock.
func startBuildHolder(t *testing.T) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	c := helperCmd("build", "OPLOCK_BLOCK=1", oplock.EnvToken+"=")
	in, err := c.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = c.Process.Kill(); _ = c.Wait() })
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("holder not ready: %q %v", line, err)
	}
	return c, in
}

// fakeClock advances only through Sleep.
type fakeClock struct {
	now     time.Time
	sleeps  int
	onSleep func(int)
}

func (c *fakeClock) clock() oplock.Clock {
	return oplock.Clock{
		Now: func() time.Time { return c.now },
		Sleep: func(_ context.Context, d time.Duration) error {
			c.sleeps++
			c.now = c.now.Add(d)
			if c.onSleep != nil {
				c.onSleep(c.sleeps)
			}
			return nil
		},
	}
}

func useClock(t *testing.T, c oplock.Clock) *bytes.Buffer {
	t.Helper()
	oldClock, oldErr := oplock.DefaultClock, oplock.Stderr
	buf := &bytes.Buffer{}
	oplock.DefaultClock, oplock.Stderr = c, buf
	t.Cleanup(func() { oplock.DefaultClock, oplock.Stderr = oldClock, oldErr })
	return buf
}

func TestOpLockDecoratorContention(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := project(t)
		holder, stdin := startBuildHolder(t)
		fc := &fakeClock{now: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}
		stderr := useClock(t, fc.clock())
		f := newLockFixture(t)

		if compat.V15() {
			// Fail fast with E460 naming the holder; no polling.
			err := f.run("build")
			d := errs.Describe(err)
			if d == nil || d.Code != "E460" || errs.ExitCodeFor(err) != 1 {
				t.Fatalf("want E460 exit 1, got %v", err)
			}
			if want := fmt.Sprintf("nself build (pid %d)", holder.Process.Pid); !strings.Contains(err.Error(), want) || !strings.Contains(d.Remediation, "wait for "+want+" or stop it") {
				t.Fatalf("holder not named: %v / %q", err, d.Remediation)
			}
			if f.ran["build"] != 0 || fc.sleeps != 0 {
				t.Fatalf("refused command ran %d times, polled %d", f.ran["build"], fc.sleeps)
			}
			return
		}
		// v1.4: wait, then proceed once the holder finishes.
		fc.onSleep = func(n int) {
			if n == 4 {
				_ = stdin.Close()
				_ = holder.Wait()
			}
		}
		if err := f.run("build"); err != nil || f.ran["build"] != 1 || fc.sleeps != 4 {
			t.Fatalf("v1.4 wait: err=%v ran=%d sleeps=%d", err, f.ran["build"], fc.sleeps)
		}
		if !strings.Contains(stderr.String(), "waiting up to 30s for nself build") {
			t.Fatalf("no wait notice: %q", stderr.String())
		}
		if !lockFileExists(dir) {
			t.Fatal("lock file missing")
		}
	})
}

func TestOpLockDecoratorV14TimeoutProceedsWithWarning(t *testing.T) {
	compattest.Set(t, false)
	project(t)
	holder, _ := startBuildHolder(t)
	fc := &fakeClock{now: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}
	stderr := useClock(t, fc.clock())
	f := newLockFixture(t)
	if err := f.run("build"); err != nil || f.ran["build"] != 1 {
		t.Fatalf("v1.4 timeout must proceed unlocked: err=%v ran=%d", err, f.ran["build"])
	}
	if fc.now.Sub(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)) < 30*time.Second {
		t.Fatalf("gave up before 30s: %v", fc.now)
	}
	if w := stderr.String(); !strings.Contains(w, "warning") || !strings.Contains(w, fmt.Sprint(holder.Process.Pid)) || !strings.Contains(w, "continuing without the lock") {
		t.Fatalf("no timeout warning: %q", w)
	}
}

func TestOpLockJSONEnvelopeOnContention(t *testing.T) {
	compattest.Set(t, true)
	project(t)
	holder, _ := startBuildHolder(t)
	f := newLockFixture(t)
	err := f.run("build", "--json")
	if errs.Describe(err) == nil || errs.Describe(err).Code != "E460" {
		t.Fatalf("want E460, got %v", err)
	}
	command, jsonMode, known := output.Invocation()
	if !known || !jsonMode || command != "build" {
		t.Fatalf("invocation not recorded for main: %q %v %v", command, jsonMode, known)
	}
	var out, errOut bytes.Buffer
	if e := output.EmitError(output.Writer{Out: &out, Err: &errOut}, command, err); e != nil {
		t.Fatal(e)
	}
	var env struct {
		Command string `json:"command"`
		Error   struct {
			Code        string `json:"code"`
			Message     string `json:"message"`
			Remediation string `json:"remediation"`
			ExitCode    int    `json:"exit_code"`
		} `json:"error"`
	}
	if e := json.Unmarshal(out.Bytes(), &env); e != nil || bytes.Count(out.Bytes(), []byte("\n{")) != 0 {
		t.Fatalf("stdout is not one envelope: %v\n%s", e, out.String())
	}
	pid := fmt.Sprint(holder.Process.Pid)
	if env.Command != "build" || env.Error.Code != "E460" || env.Error.ExitCode != 1 ||
		!strings.Contains(env.Error.Message, pid) || !strings.Contains(env.Error.Remediation, "nself build (pid "+pid+")") {
		t.Fatalf("envelope wrong: %+v", env)
	}
	if errOut.Len() != 0 {
		t.Fatalf("EmitError wrote to stderr: %q", errOut.String())
	}
}

func TestOpLockReentrancy(t *testing.T) {
	compattest.Set(t, true)
	project(t)
	// A child nself spawned by a lock-holding command inherits the token and
	// runs without waiting (v1.5 would fail it at once otherwise).
	f := newLockFixture(t)
	if err := f.run("nest"); err != nil {
		t.Fatalf("child with the inherited token must run: %v", err)
	}
	// The same child without the token is refused with E460.
	err := newLockFixture(t).run("nest-bare")
	if d := errs.Describe(err); d == nil || d.Code != "E460" {
		t.Fatalf("child without the token must fail with E460, got %v", err)
	}
	// The token is exported only while the lock is held.
	if v, ok := os.LookupEnv(oplock.EnvToken); ok {
		t.Fatalf("%s leaked after the command: %q", oplock.EnvToken, v)
	}
}

func TestOpLockWatchRelease(t *testing.T) {
	compattest.Set(t, true)
	project(t)
	// Without --watch the start body still holds the lock: a concurrent write
	// command from another process (no token) is refused with E460.
	err := newLockFixture(t).run("start")
	if d := errs.Describe(err); d == nil || d.Code != "E460" {
		t.Fatalf("control: config set during plain start must be refused, got %v", err)
	}
	// With --watch the lock is released before the loop: the same child
	// proceeds while start is still running.
	if err := newLockFixture(t).run("start", "--watch"); err != nil {
		t.Fatalf("config set during start --watch must proceed: %v", err)
	}
}

func TestOpLockReadCommands(t *testing.T) {
	// Registry-wide: the lock is taken exactly for write/remote/destructive
	// document commands; read, stream and interactive never take it.
	reattachRealTree()
	resetRegistryCache()
	t.Cleanup(resetRegistryCache)
	reg, err := commandRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reads, takes := 0, 0
	for _, c := range reg.Commands {
		want := canon.Rank(c.SideEffect) >= canon.Rank(canon.SideEffectWrite) && c.Output == canon.OutputDocument
		got := oplock.Takes(c.SideEffect, c.Output)
		if got != want {
			t.Errorf("%s: Takes(%s,%s)=%v, want %v", c.Path, c.SideEffect, c.Output, got, want)
		}
		if c.SideEffect == canon.SideEffectRead {
			reads++
			if got {
				t.Errorf("%s is read and must never take the lock", c.Path)
			}
		}
		if got {
			takes++
		}
	}
	if reads == 0 || takes == 0 {
		t.Fatalf("registry looks empty: %d read, %d locking", reads, takes)
	}
	// Flag escalation on the real tree: `start --watch` is a stream.
	cmd := parseReal(t, "start")
	if err := cmd.ParseFlags([]string{"--watch"}); err != nil {
		t.Fatal(err)
	}
	e, _ := reg.Lookup("start")
	if side, out := oplockcmd.Effective(cmd, e); oplock.Takes(side, out) {
		t.Errorf("start --watch must not take the lock (class %s/%s)", side, out)
	}

	// Fixture behaviour: read and stream commands never touch the lock file;
	// an escalating flag does; outside a project nothing is created.
	for _, v := range []string{"1", "0"} {
		t.Setenv(compat.EnvVar, v)
		dir := project(t)
		f := newLockFixture(t)
		for _, args := range [][]string{{"status"}, {"logs"}, {"plug"}} {
			if err := f.run(args...); err != nil {
				t.Fatal(err)
			}
			if lockFileExists(dir) {
				t.Fatalf("%v created the lock file", args)
			}
		}
		if err := f.run("plug", "--apply"); err != nil || !lockFileExists(dir) {
			t.Fatalf("plug --apply must take the lock: err=%v exists=%v", err, lockFileExists(dir))
		}
		bare := t.TempDir()
		t.Chdir(bare)
		if err := newLockFixture(t).run("build"); err != nil || lockFileExists(bare) {
			t.Fatalf("outside a project: err=%v lock=%v", err, lockFileExists(bare))
		}
	}
}

func TestOpLockReleasedOnErrorAndPanic(t *testing.T) {
	compattest.Set(t, true)
	dir := project(t)
	f := newLockFixture(t)
	free := func(when string) {
		l, err := oplock.Acquire(context.Background(), dir, oplock.Opts{Command: "probe"})
		if err != nil {
			t.Fatalf("lock still held after %s: %v", when, err)
		}
		l.Release()
	}
	t.Setenv("OPLOCK_MODE", "error")
	if err := f.run("build"); err == nil {
		t.Fatal("body error lost")
	}
	free("an error return")
	t.Setenv("OPLOCK_MODE", "panic")
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed by the decorator")
			}
		}()
		_ = f.run("build")
	}()
	free("a panic")
}
