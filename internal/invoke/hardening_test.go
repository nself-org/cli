package invoke

// Purpose: fix round 1 of P7-SURF-24 (Opus adversarial review): free-form argv
// on a plugin parent (M1), the process group (S1), exact request members (S2)
// and secrets in the wrong slot (nit).
// Constraints: each test fails on the code before the fix (see the mutation
// script qa/P7-SURF-24-09-mutation-fix1.sh).

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestInvokeFreeFormArgvNeedsLeaf is review finding M1: a mounted plugin node
// parses no flags, so the child reads the first word of a free-form argv as a
// subcommand and runs a node the exposure rules never checked.
func TestInvokeFreeFormArgvNeedsLeaf(t *testing.T) {
	useStub(t)
	// A refusal never starts a child: a binary that would fail proves it.
	t.Setenv(SelfExecOverrideEnv, filepath.Join(t.TempDir(), "must-not-run"))
	reg := fixtureRegistry(t)
	for _, doc := range []string{`{"argv":["purge","--all"]}`, `{"argv":["leak"]}`, `{"argv":["x"]}`} {
		for _, path := range []string{"pwn status", "pwn"} {
			res, err := Invoke(context.Background(), reg, path, req(t, doc), Options{})
			if err != nil {
				t.Fatal(err)
			}
			e := errorOf(t, res)
			why, _ := e["cause"].(string)
			if e["code"] != "E421" || !strings.Contains(why, "leaf") || res.ExitCode != 1 {
				t.Errorf("%s %s: want E421 naming the leaf rule, got %v exit=%d", path, doc, e, res.ExitCode)
			}
		}
	}
	// The children themselves stay refused by their own exposure.
	for path, want := range map[string]string{"pwn status purge": "no gate", "pwn status leak": "cli-only"} {
		res, _ := Invoke(context.Background(), reg, path, Request{}, Options{})
		if e := errorOf(t, res); e["code"] != "E421" || !strings.Contains(e["cause"].(string), want) {
			t.Errorf("%s: %v", path, e)
		}
	}
	// Structure: the parent is not a free-form leaf, the plain plugin command is.
	if FreeFormArgv(reg, mustCmd(t, reg, "pwn status")) || !FreeFormArgv(reg, mustCmd(t, reg, "demo")) {
		t.Error("FreeFormArgv must hold for a leaf plugin command only")
	}
	if IsLeaf(reg, mustCmd(t, reg, "pwn")) || !IsLeaf(reg, mustCmd(t, reg, "pwn status leak")) || !IsLeaf(nil, mustCmd(t, reg, "pwn")) {
		t.Error("IsLeaf is wrong")
	}
	props := func(path string) map[string]any {
		return ParamsSchemaIn(reg, mustCmd(t, reg, path))["properties"].(map[string]any)
	}
	if _, has := props("pwn status")["argv"]; has {
		t.Error("a plugin parent must not advertise free-form argv")
	}
	if _, has := props("demo")["argv"]; !has {
		t.Error("a leaf plugin command advertises free-form argv")
	}
	// A leaf still runs with its argv.
	useStub(t)
	res, err := Invoke(context.Background(), reg, "demo", req(t, `{"argv":["a","b"]}`), Options{})
	var d stubData
	if err != nil {
		t.Fatal(err)
	}
	decodeInto(t, res.Stdout, &d)
	if !reflect.DeepEqual(d.Data.Argv, []string{"demo", "--json", "a", "b"}) {
		t.Errorf("leaf argv: %q (%s)", d.Data.Argv, res.Stdout)
	}
}

func decodeInto(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
}

// TestInvokeFreeFormArgvRootFlags is the review's nit: the child's plugin
// proxy drops root persistent flags, so argv is not passed verbatim and such
// items are refused instead.
func TestInvokeFreeFormArgvRootFlags(t *testing.T) {
	useStub(t)
	reg := fixtureRegistry(t)
	cmd := mustCmd(t, reg, "demo")
	for _, item := range []string{"--no-monorepo", "--no-monorepo=1", "--json", "--no-deprecation-warnings", "--help"} {
		err := CheckFreeForm(reg, cmd, Request{Argv: []string{"x", item}})
		wantCode(t, err, "E420", item[:strings.IndexAny(item+"=", "=")])
	}
	for _, argv := range [][]string{{"x", "--", "--no-monorepo"}, {"--other", "--no-monorepox"}, {"--name=--no-monorepo"}} {
		if err := CheckFreeForm(reg, cmd, Request{Argv: argv}); err != nil {
			t.Errorf("%q must pass: %v", argv, err)
		}
	}
	res, _ := Invoke(context.Background(), reg, "demo", req(t, `{"argv":["--no-monorepo","x"]}`), Options{})
	if e := errorOf(t, res); e["code"] != "E420" {
		t.Errorf("Invoke: %v", e)
	}
}

// TestDecodeRequestExactMembers is review finding S2: encoding/json folds case
// and Unicode in member names and keeps the last duplicate.
func TestDecodeRequestExactMembers(t *testing.T) {
	bad := map[string]string{
		"upper case args":        `{"ARGS":["a"]}`,
		"mixed case flags":       `{"Flags":{"x":true}}`,
		"long s fold":            "{\"argſ\":[\"a\"]}",
		"kelvin fold":            "{\"confirm\":\"\",\"Key\":1}",
		"duplicate args":         `{"args":["safe"],"args":["evil"]}`,
		"case duplicate":         `{"args":["safe"],"ARGS":["evil"]}`,
		"escaped duplicate":      `{"args":["safe"],"args":["evil"]}`,
		"duplicate flags member": `{"flags":{"a":1},"flags":{"b":2}}`,
		"duplicate flag name":    `{"flags":{"name":"a","name":"b"}}`,
		"duplicate in flags":     `{"args":["w"],"flags":{"on":true,"tags":["a"],"on":false}}`,
		"duplicate argv":         `{"argv":["a"],"argv":["b"]}`,
		"duplicate confirm":      `{"confirm":"","confirm":""}`,
	}
	for name, doc := range bad {
		_, err := DecodeRequest([]byte(doc))
		if err == nil {
			t.Errorf("%s: %q must be E420", name, doc)
			continue
		}
		wantCode(t, err, "E420")
	}
	good := []string{`{}`, `{"args":["a"],"flags":{"x":1,"X":2}}`, `{"flags":{},"args":[],"argv":[],"confirm":""}`,
		`{"flags":{"l":["a","a"],"o":{"args":1}}}`, `{"args":["a"]}`}
	for _, doc := range good {
		if _, err := DecodeRequest([]byte(doc)); err != nil {
			t.Errorf("%q must decode: %v", doc, err)
		}
	}
	// The flag name case is kept: X and x are two flags, not a fold.
	r := req(t, `{"flags":{"x":1,"X":2}}`)
	if len(r.Flags) != 2 {
		t.Errorf("flags %v", r.Flags)
	}
}

// TestRedactionFailsClosed is the review's second nit: a secret sent in a slot
// the command does not declare still reaches the request id of the refusal.
func TestRedactionFailsClosed(t *testing.T) {
	reg := fixtureRegistry(t)
	cmd := mustCmd(t, reg, "fx secretset")
	a := RequestID("fx secretset", cmd, Request{Args: []string{"k", "v", "extra-one"}, Flags: map[string]any{"nope": "s1"}})
	b := RequestID("fx secretset", cmd, Request{Args: []string{"k", "v", "extra-two"}, Flags: map[string]any{"nope": "s2"}})
	if a != b {
		t.Error("two requests that differ only in an undeclared slot must share an id")
	}
	red := RedactRequest(cmd, Request{Args: []string{"k", "v", "z"}, Flags: map[string]any{"nope": "s1", "tok": "t"}})
	if red.Args[2] != Redacted || red.Flags["nope"] != Redacted {
		t.Errorf("undeclared slots must be redacted: %+v", red)
	}
	if red.Args[0] != "k" {
		t.Errorf("a declared plain arg stays visible: %+v", red)
	}
	vals := SecretValues(cmd, Request{Args: []string{"k", "v", "tail-secret"}, Flags: map[string]any{"nope": "flag-secret"}})
	if !reflect.DeepEqual(sortedCopy(vals), []string{"flag-secret", "tail-secret", "v"}) {
		t.Errorf("SecretValues %q", vals)
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// gone reports whether pid is no longer a live process (a zombie counts as gone).
func gone(pid int) bool {
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	st := strings.TrimSpace(string(out))
	return err != nil || st == "" || strings.HasPrefix(st, "Z")
}

func startGrandchild(ctx context.Context, t *testing.T, timeout time.Duration) (int, time.Duration) {
	t.Helper()
	pidfile := filepath.Join(t.TempDir(), "gc.pid")
	useStub(t, "STUBCLI_MODE", "grandchild", "STUBCLI_PIDFILE", pidfile)
	old := killDelay
	killDelay = 300 * time.Millisecond
	t.Cleanup(func() { killDelay = old })
	start := time.Now()
	_, err := Exec(ctx, Spec{Argv: []string{"x"}, Transport: TransportMCP, Timeout: timeout})
	took := time.Since(start)
	if timeout == 0 && err == nil {
		t.Fatal("a cancelled Exec returns the context error")
	}
	raw, rerr := os.ReadFile(pidfile)
	if rerr != nil {
		t.Fatalf("the stub never started its grandchild (%v), exec err=%v", rerr, err)
	}
	pid, _ := strconv.Atoi(string(raw))
	return pid, took
}

// TestExecKillsGrandchildren is review finding S1: the child leads its own
// process group and the whole group dies on timeout and on cancellation, even
// a grandchild that ignores SIGTERM.
func TestExecKillsGrandchildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are not used on Windows")
	}
	check := func(name string, pid int) {
		t.Helper()
		for i := 0; i < 40 && !gone(pid); i++ {
			time.Sleep(50 * time.Millisecond)
		}
		if !gone(pid) {
			_ = exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
			t.Errorf("%s: the grandchild %d outlived the child", name, pid)
		}
	}
	pid, took := startGrandchild(context.Background(), t, time.Second)
	if took > 4*time.Second {
		t.Errorf("timeout took %s", took)
	}
	check("timeout", pid)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pid, _ = startGrandchild(ctx, t, 0)
	check("cancel", pid)
}
