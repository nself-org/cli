package invoke

// Purpose: tests of the confirmation gate (gate.go, confirm.go) with the stub
// child and a fixture registry of gated commands.
// Constraints: the stub records every argv in STUBCLI_ARGVLOG, which is how a
// test proves what ran and what did not; time is an injected clock.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/spf13/cobra"
)

const gateCanon = `
schema_version: 1
verbs: [gx]
commands:
  gx: {canon: core}
  gx read: {side_effect: read}
  gx put: {side_effect: write, secret_args: [value]}
  gx plain:
    side_effect: write
    confirm: {flags: [yes]}
  gx push:
    side_effect: remote
    confirm: {flags: [yes]}
  gx wipe:
    side_effect: destructive
    confirm: {flags: [yes, force]}
  gx apply:
    side_effect: write
    confirm: {flags: [force], plan: {flag: plan, id_flag: plan-id}}
  gx purge:
    side_effect: destructive
    confirm: {flags: [yes], plan: {flag: plan, id_flag: plan-id}}
  gx scan:
    side_effect: read
    confirm: {flags: [yes]}
    flags:
      prune: {side_effect: destructive}
  gx bare: {side_effect: destructive}
  help: {canon: builtin, side_effect: read}
`

// gateRegistry builds the registry of the gate fixture.
func gateRegistry(t testing.TB) *cmdregistry.Registry {
	t.Helper()
	file, err := canon.Parse([]byte(gateCanon))
	if err != nil {
		t.Fatalf("gate canon: %v", err)
	}
	root := &cobra.Command{Use: "nself", Short: "fixture"}
	root.PersistentFlags().Bool("json", false, "json")
	root.PersistentFlags().Bool("no-monorepo", false, "no monorepo")
	gx := &cobra.Command{Use: "gx", Short: "gate fixture hub"}
	leaf := func(use string, bools []string, strs []string) *cobra.Command {
		c := &cobra.Command{Use: use, Short: use, RunE: noop}
		for _, b := range bools {
			c.Flags().Bool(b, false, b)
		}
		for _, s := range strs {
			c.Flags().String(s, "", s)
		}
		return c
	}
	gx.AddCommand(
		leaf("read", nil, []string{"name"}),
		leaf("put <key> <value>", nil, []string{"db-password", "note"}),
		leaf("plain", []string{"yes"}, []string{"target"}),
		leaf("push", []string{"yes"}, []string{"target"}),
		leaf("wipe [name]", []string{"yes", "force"}, []string{"target"}),
		leaf("apply", []string{"force", "plan"}, []string{"plan-id", "target"}),
		leaf("purge", []string{"yes", "plan"}, []string{"plan-id", "target"}),
		leaf("scan", []string{"yes", "prune"}, nil),
		leaf("bare", nil, []string{"target"}),
	)
	root.AddCommand(gx)
	root.InitDefaultHelpCmd()
	types := map[string]any{}
	for _, p := range []string{"read", "put", "plain", "push", "wipe", "apply", "purge", "scan", "bare"} {
		types["gx "+p] = struct{}{}
	}
	reg, err := cmdregistry.Build(root, file, types, cmdregistry.BuildOptions{V15: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return reg
}

// fakeClock is the injected Guard clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// gateEnv is one test's stub, Guard, clock, project dir and logs.
type gateEnv struct {
	t       *testing.T
	reg     *cmdregistry.Registry
	guard   *Guard
	clock   *fakeClock
	dir     string
	audit   string
	argvLog string
	surface string
}

// newGateEnv points the invoker at the stub (echo mode unless kv says
// otherwise) with an argv log, a Guard and a clock.
func newGateEnv(t *testing.T, kv ...string) *gateEnv {
	t.Helper()
	base := t.TempDir()
	e := &gateEnv{t: t, reg: gateRegistry(t), dir: filepath.Join(base, "proj"),
		audit: filepath.Join(base, "home", ".nself", "logs", "machine-audit.jsonl"), argvLog: filepath.Join(base, "argv.log")}
	if err := os.MkdirAll(e.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	useStub(t, append([]string{"STUBCLI_ARGVLOG", e.argvLog}, kv...)...)
	e.clock = &fakeClock{t: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
	e.guard = NewGuard(e.audit)
	e.guard.Now = e.clock.now
	return e
}

// call invokes path with the request document doc.
func (e *gateEnv) call(path, doc string) Result {
	e.t.Helper()
	res, err := Invoke(context.Background(), e.reg, path, req(e.t, doc), Options{Dir: e.dir, Guard: e.guard, Surface: e.surface})
	if err != nil {
		e.t.Fatalf("%s: %v", path, err)
	}
	return res
}

// argvs returns every argv the stub recorded, in order.
func (e *gateEnv) argvs() [][]string {
	e.t.Helper()
	f, err := os.Open(e.argvLog)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		e.t.Fatal(err)
	}
	defer f.Close()
	var out [][]string
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var a []string
		if err := json.Unmarshal(sc.Bytes(), &a); err != nil {
			e.t.Fatalf("argv log line %q: %v", sc.Text(), err)
		}
		out = append(out, a)
	}
	return out
}

// auditLines returns the parsed audit lines (nil when there is no file).
func (e *gateEnv) auditLines() []map[string]any {
	e.t.Helper()
	b, err := os.ReadFile(e.audit)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		e.t.Fatal(err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			e.t.Fatalf("audit line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

// confirmationOf decodes a confirmation envelope.
func confirmationOf(t *testing.T, res Result) Confirmation {
	t.Helper()
	var doc struct {
		Data Confirmation `json:"data"`
	}
	if err := json.Unmarshal(res.Stdout, &doc); err != nil || !doc.Data.ConfirmationRequired {
		t.Fatalf("want a confirmation envelope, got %v: %s", err, res.Stdout)
	}
	if res.ExitCode != 0 || res.ErrorCode != "" {
		t.Errorf("a confirmation is a success: exit=%d code=%q", res.ExitCode, res.ErrorCode)
	}
	return doc.Data
}

func hasArg(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}

var hex64 = confirmPattern

func TestGateBySideEffect(t *testing.T) {
	e := newGateEnv(t)
	// read runs and is not audited.
	if res := e.call("gx read", `{}`); res.ExitCode != 0 || res.ErrorCode != "" {
		t.Fatalf("read: %+v", res)
	}
	if len(e.argvs()) != 1 || e.auditLines() != nil {
		t.Errorf("read: argvs=%v audit=%v", e.argvs(), e.auditLines())
	}
	// write runs with no consent flag and writes one audit line.
	if res := e.call("gx put", `{"args":["K","v"]}`); res.ExitCode != 0 || res.ErrorCode != "" {
		t.Fatalf("write: %+v", res)
	}
	if lines := e.auditLines(); len(lines) != 1 || lines[0]["decision"] != "run" || lines[0]["side_effect"] != "write" {
		t.Errorf("write audit: %v", lines)
	}
	// write with a confirm block and no plan form runs, no consent flags.
	e.call("gx plain", `{}`)
	if got := e.argvs()[2]; !reflect.DeepEqual(got, []string{"gx", "plain", "--json"}) {
		t.Errorf("plain write argv %q", got)
	}
	// remote and destructive without a plan form: confirmation, nothing run.
	ran := len(e.argvs())
	for _, c := range []struct{ path, side string }{{"gx push", "remote"}, {"gx wipe", "destructive"}} {
		res := e.call(c.path, `{}`)
		conf := confirmationOf(t, res)
		if conf.Kind != "request" || conf.SideEffect != c.side || !hex64.MatchString(conf.Confirm) || conf.Plan != nil || conf.Reason == "" {
			t.Errorf("%s: %+v", c.path, conf)
		}
		if doc := decodeDoc(t, res.Stdout); doc["command"] != c.path || doc["error"] != nil {
			t.Errorf("%s: envelope %s", c.path, res.Stdout)
		}
	}
	if len(e.argvs()) != ran {
		t.Errorf("a confirmation must not start a child: %v", e.argvs()[ran:])
	}
	// flag escalation: scan is a read until --prune names a destructive effect.
	if res := e.call("gx scan", `{}`); res.ExitCode != 0 || res.ErrorCode != "" {
		t.Errorf("scan: %+v", res)
	}
	if conf := confirmationOf(t, e.call("gx scan", `{"flags":{"prune":true}}`)); conf.SideEffect != "destructive" || conf.Kind != "request" {
		t.Errorf("escalated scan: %+v", conf)
	}
	// destructive without a confirm block is never exposed.
	res := e.call("gx bare", `{}`)
	if e := errorOf(t, res); e["code"] != "E421" || !strings.Contains(e["cause"].(string), "no gate") {
		t.Errorf("bare: %v", e)
	}
	// a confirm on a command without a confirm block stays E420.
	if res := e.call("gx read", `{"confirm":"`+strings.Repeat("a", 64)+`"}`); errorOf(t, res)["code"] != "E420" {
		t.Errorf("confirm on read: %s", res.Stdout)
	}
	// audit: put(run), push, wipe, scan+prune (confirmations), bare (refused), read+confirm (refused, read still logged).
	var decisions []string
	for _, l := range e.auditLines() {
		decisions = append(decisions, l["decision"].(string))
	}
	want := []string{"run", "run", "confirmation_required", "confirmation_required", "confirmation_required", "refused", "refused"}
	if !reflect.DeepEqual(decisions, want) {
		t.Errorf("audit decisions %v, want %v", decisions, want)
	}
}

func TestGateNilGuardFailsClosed(t *testing.T) {
	e := newGateEnv(t)
	call := func(path, doc string) Result {
		res, err := Invoke(context.Background(), e.reg, path, req(t, doc), Options{Dir: e.dir})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	for _, p := range []string{"gx push", "gx wipe", "gx purge"} {
		if x := errorOf(t, call(p, `{}`)); x["code"] != "E421" || !strings.Contains(x["cause"].(string), "no gate") {
			t.Errorf("%s without a Guard: %v", p, x)
		}
	}
	if res := call("gx put", `{"args":["K","v"]}`); res.ExitCode != 0 || res.ErrorCode != "" {
		t.Errorf("a write runs without a Guard: %+v", res)
	}
	if _, err := os.Stat(e.audit); !os.IsNotExist(err) {
		t.Error("a nil Guard writes no audit")
	}
	if n := len(e.argvs()); n != 1 {
		t.Errorf("only the write ran, got %d children", n)
	}
}

func shaHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestGatePlanForm(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(state, []byte("A"), 0o600); err != nil {
		t.Fatal(err)
	}
	id := shaHex("A")
	e := newGateEnv(t, "STUBCLI_MODE", "plan", "STUBCLI_STATE", state, "STUBCLI_REQCONF", "0")

	// requires_confirmation=false on a write: one plan exec, then one run with the id.
	res := e.call("gx apply", `{"flags":{"target":"x"}}`)
	if res.ExitCode != 0 || res.ErrorCode != "" {
		t.Fatalf("apply: %+v %s", res, res.Stdout)
	}
	av := e.argvs()
	if len(av) != 2 || !hasArg(av[0], "--plan=true") || hasArg(av[0], "--plan-id="+id) {
		t.Fatalf("plan exec: %q", av)
	}
	if !reflect.DeepEqual(av[1], []string{"gx", "apply", "--json", "--target=x", "--plan-id=" + id}) {
		t.Errorf("apply argv %q (no consent flags, no --plan)", av[1])
	}
	if lines := e.auditLines(); len(lines) != 1 || lines[0]["decision"] != "run" || lines[0]["confirm_kind"] != nil {
		t.Errorf("audit after an unconfirmed plan run: %v", lines)
	}

	// requires_confirmation=true: a kind plan confirmation, only the plan ran.
	t.Setenv("STUBCLI_REQCONF", "1")
	if err := os.Remove(e.argvLog); err != nil {
		t.Fatal(err)
	}
	conf := confirmationOf(t, e.call("gx apply", `{"flags":{"target":"x"},"args":[]}`))
	plan, _ := conf.Plan.(map[string]any)
	if conf.Kind != "plan" || conf.Confirm != id || plan["plan_id"] != id || conf.SideEffect != "write" {
		t.Fatalf("plan confirmation: %+v", conf)
	}
	if len(e.argvs()) != 1 || !hasArg(e.argvs()[0], "--plan=true") {
		t.Errorf("only the plan exec runs: %q", e.argvs())
	}

	// confirmed: consent flags and the id, no --plan.
	res = e.call("gx apply", `{"flags":{"target":"x"},"confirm":"`+id+`"}`)
	if res.ExitCode != 0 || res.ErrorCode != "" {
		t.Fatalf("confirmed apply: %+v", res)
	}
	if got := e.argvs()[1]; !reflect.DeepEqual(got, []string{"gx", "apply", "--json", "--target=x", "--force=true", "--plan-id=" + id}) {
		t.Errorf("confirmed argv %q", got)
	}
	lines := e.auditLines()
	if last := lines[len(lines)-1]; last["decision"] != "run" || last["confirm_kind"] != "plan" || last["exit_code"] != float64(0) {
		t.Errorf("confirmed audit: %v", last)
	}

	// state drift: the child's E450 passes through unchanged.
	if err := os.WriteFile(state, []byte("B"), 0o600); err != nil {
		t.Fatal(err)
	}
	res = e.call("gx apply", `{"flags":{"target":"x"},"confirm":"`+id+`"}`)
	if x := errorOf(t, res); x["code"] != "E450" || res.ExitCode != 1 || res.ErrorCode != "" {
		t.Errorf("drift: %v exit=%d invokerCode=%q", x, res.ExitCode, res.ErrorCode)
	}
	lines = e.auditLines()
	if last := lines[len(lines)-1]; last["error_code"] != "E450" || last["exit_code"] != float64(1) {
		t.Errorf("drift audit: %v", last)
	}

	// a destructive plan form always asks, even when the plan says no.
	t.Setenv("STUBCLI_REQCONF", "0")
	if conf := confirmationOf(t, e.call("gx purge", `{}`)); conf.Kind != "plan" || conf.SideEffect != "destructive" {
		t.Errorf("purge: %+v", conf)
	}
	nilGuard, _ := Invoke(context.Background(), e.reg, "gx purge", Request{}, Options{Dir: e.dir})
	if errorOf(t, nilGuard)["code"] != "E421" {
		t.Errorf("a destructive plan form without a Guard fails closed: %s", nilGuard.Stdout)
	}

	// the plan exec's own error envelope passes through byte for byte.
	childErr := `{"schema_version":"1","command":"gx apply","error":{"code":"E999","message":"no plan","exit_code":1,"class":"user"}}`
	t.Setenv("STUBCLI_MODE", "raw")
	t.Setenv("STUBCLI_STDOUT", childErr)
	t.Setenv("STUBCLI_EXIT", "1")
	if res := e.call("gx apply", `{}`); string(res.Stdout) != childErr || res.ExitCode != 1 || res.ErrorCode != "" {
		t.Errorf("plan error passthrough: %s exit=%d", res.Stdout, res.ExitCode)
	}

	// a plan document without usable fields is E422 with a clear cause.
	for name, doc := range map[string]string{
		"plan_id":               `{"schema_version":"1","command":"x","data":{"requires_confirmation":false}}`,
		"64 lowercase hex":      `{"schema_version":"1","command":"x","data":{"plan_id":"zz","requires_confirmation":false}}`,
		"requires_confirmation": `{"schema_version":"1","command":"x","data":{"plan_id":"` + id + `"}}`,
		"data object":           `{"schema_version":"1","command":"x","data":null}`,
	} {
		t.Setenv("STUBCLI_STDOUT", doc)
		t.Setenv("STUBCLI_EXIT", "")
		x := errorOf(t, e.call("gx apply", `{}`))
		if x["code"] != "E422" || !strings.Contains(x["cause"].(string), name) {
			t.Errorf("%s: %v", name, x)
		}
	}
}

func TestGateRequestNonce(t *testing.T) {
	e := newGateEnv(t)
	first := e.call("gx wipe", `{"args":["n1"],"flags":{"target":"t"}}`)
	conf := confirmationOf(t, first)
	if conf.Kind != "request" || !hex64.MatchString(conf.Confirm) || conf.Confirm == first.RequestID {
		t.Fatalf("confirmation: %+v id=%s", conf, first.RequestID)
	}
	if len(e.argvs()) != 0 {
		t.Fatalf("no apply argv before the confirmation: %v", e.argvs())
	}
	res := e.call("gx wipe", `{"args":["n1"],"flags":{"target":"t"},"confirm":"`+conf.Confirm+`"}`)
	if res.ExitCode != 0 || res.ErrorCode != "" {
		t.Fatalf("confirmed: %+v %s", res, res.Stdout)
	}
	want := []string{"gx", "wipe", "--json", "--target=t", "--yes=true", "--force=true", "--", "n1"}
	if av := e.argvs(); len(av) != 1 || !reflect.DeepEqual(av[0], want) {
		t.Errorf("apply argv %q, want %q", av, want)
	}
	// the nonce never reaches the audit file, the request id or the response.
	raw, _ := os.ReadFile(e.audit)
	if strings.Contains(string(raw), conf.Confirm) || strings.Contains(res.RequestID, conf.Confirm) || strings.Contains(string(res.Stdout), conf.Confirm) {
		t.Error("the nonce leaked")
	}
	lines := e.auditLines()
	if len(lines) != 2 || lines[0]["decision"] != "confirmation_required" || lines[0]["confirm_kind"] != "request" ||
		lines[0]["exit_code"] != nil || lines[1]["decision"] != "run" || lines[1]["confirm_kind"] != "request" || lines[1]["exit_code"] != float64(0) {
		t.Errorf("audit lines: %v", lines)
	}
	if lines[0]["request_id"] != lines[1]["request_id"] {
		t.Error("both lines name the same request")
	}
	// two nonces for one request are independent; each is single use.
	a := confirmationOf(t, e.call("gx push", `{}`)).Confirm
	b := confirmationOf(t, e.call("gx push", `{}`)).Confirm
	if a == b {
		t.Fatal("nonces repeat")
	}

	t.Run("store is bounded", func(t *testing.T) {
		old := maxLiveNonces
		maxLiveNonces = 8
		defer func() { maxLiveNonces = old }()
		g := NewGuard("")
		clock := &fakeClock{t: time.Unix(1000, 0)}
		g.Now = clock.now
		var digests [][32]byte
		var nonces []string
		for i := 0; i < 20; i++ {
			d, _ := requestDigest("gx push", Request{Args: []string{string(rune('a' + i))}})
			n, err := g.issue(d)
			if err != nil {
				t.Fatal(err)
			}
			digests, nonces = append(digests, d), append(nonces, n)
			if g.liveNonces() > maxLiveNonces {
				t.Fatalf("store holds %d entries", g.liveNonces())
			}
		}
		if g.consume(nonces[0], digests[0]) {
			t.Error("the oldest entry was evicted")
		}
		if !g.consume(nonces[19], digests[19]) {
			t.Error("the newest entry is live")
		}
		// expired entries go first.
		if _, err := g.issue(digests[0]); err != nil {
			t.Fatal(err)
		}
		clock.advance(nonceTTL + time.Second)
		fresh, _ := g.issue(digests[0])
		if g.liveNonces() != 1 || !g.consume(fresh, digests[0]) {
			t.Errorf("expired entries are dropped first: %d live", g.liveNonces())
		}
	})
}

func TestGateNonceReuseExpiryMismatch(t *testing.T) {
	e := newGateEnv(t)
	issue := func(path, doc string) string {
		return confirmationOf(t, e.call(path, doc)).Confirm
	}
	withConfirm := func(doc, nonce string) string {
		if doc == "{}" {
			return `{"confirm":"` + nonce + `"}`
		}
		return strings.TrimSuffix(doc, "}") + `,"confirm":"` + nonce + `"}`
	}
	wantE424 := func(name string, res Result) {
		t.Helper()
		x := errorOf(t, res)
		if x["code"] != "E424" || res.ErrorCode != "E424" || res.ExitCode != 4 {
			t.Errorf("%s: want E424 exit 4, got %v exit=%d", name, x, res.ExitCode)
		}
		if strings.Contains(string(res.Stdout), "confirm\":\"") {
			t.Errorf("%s: the envelope echoes a confirm value", name)
		}
	}
	const doc = `{"flags":{"target":"a"}}`

	// reuse.
	n := issue("gx push", doc)
	if res := e.call("gx push", withConfirm(doc, n)); res.ExitCode != 0 {
		t.Fatalf("first use: %s", res.Stdout)
	}
	ran := len(e.argvs())
	wantE424("reuse", e.call("gx push", withConfirm(doc, n)))

	// expiry, with the injected clock: valid just before 10 minutes, dead at 10.
	n = issue("gx push", doc)
	e.clock.advance(10*time.Minute - time.Second)
	if res := e.call("gx push", withConfirm(doc, n)); res.ExitCode != 0 {
		t.Errorf("a nonce is valid until its TTL: %s", res.Stdout)
	}
	ran++
	n = issue("gx push", doc)
	e.clock.advance(10 * time.Minute)
	wantE424("expired", e.call("gx push", withConfirm(doc, n)))

	// a different request, and the burn: the right request after a mismatch also fails.
	n = issue("gx push", doc)
	wantE424("different flag", e.call("gx push", withConfirm(`{"flags":{"target":"b"}}`, n)))
	wantE424("spent by the mismatch", e.call("gx push", withConfirm(doc, n)))
	n = issue("gx wipe", `{"args":["x"]}`)
	wantE424("different args", e.call("gx wipe", withConfirm(`{"args":["y"]}`, n)))
	n = issue("gx push", doc)
	wantE424("different command", e.call("gx wipe", withConfirm(`{}`, n)))

	// a locally computed value, and an unknown one.
	first := e.call("gx push", doc)
	wantE424("request id", e.call("gx push", withConfirm(doc, first.RequestID)))
	wantE424("unknown", e.call("gx push", withConfirm(doc, strings.Repeat("0", 64))))
	wantE424("digest of the request", e.call("gx push", withConfirm(doc, shaHex(doc))))
	if len(e.argvs()) != ran {
		t.Errorf("a refused confirmation starts no child: %d children, want %d", len(e.argvs()), ran)
	}

	t.Run("one nonce, many concurrent calls, one run", func(t *testing.T) {
		n := issue("gx push", doc)
		before := len(e.argvs())
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok, refused := 0, 0
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res, err := Invoke(context.Background(), e.reg, "gx push", req(t, withConfirm(doc, n)), Options{Dir: e.dir, Guard: e.guard})
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil && res.ExitCode == 0 && res.ErrorCode == "":
					ok++
				case err == nil && res.ErrorCode == "E424":
					refused++
				default:
					t.Errorf("unexpected: %v %+v", err, res)
				}
			}()
		}
		wg.Wait()
		if ok != 1 || refused != 15 || len(e.argvs())-before != 1 {
			t.Errorf("ok=%d refused=%d children=%d, want exactly one run", ok, refused, len(e.argvs())-before)
		}
	})
}

func TestGateCallerConsentFlag(t *testing.T) {
	e := newGateEnv(t)
	id := strings.Repeat("a", 64)
	cases := []struct{ path, doc string }{
		{"gx wipe", `{"flags":{"yes":true}}`},
		{"gx wipe", `{"flags":{"force":true}}`},
		{"gx wipe", `{"flags":{"yes":false}}`},
		{"gx push", `{"flags":{"yes":true}}`},
		{"gx plain", `{"flags":{"yes":true}}`},
		{"gx scan", `{"flags":{"yes":true}}`},
		{"gx apply", `{"flags":{"force":true}}`},
		{"gx apply", `{"flags":{"plan":true}}`},
		{"gx apply", `{"flags":{"plan-id":"` + id + `"}}`},
		{"gx purge", `{"flags":{"plan-id":"` + id + `"},"confirm":"` + id + `"}`},
	}
	for _, c := range cases {
		res := e.call(c.path, c.doc)
		x := errorOf(t, res)
		if x["code"] != "E425" || res.ErrorCode != "E425" || res.ExitCode != 1 {
			t.Errorf("%s %s: %v exit=%d", c.path, c.doc, x, res.ExitCode)
		}
	}
	if len(e.argvs()) != 0 {
		t.Errorf("E425 starts no child: %v", e.argvs())
	}
	// the Guard-less path refuses the same way, before the fail-closed rule.
	res, _ := Invoke(context.Background(), e.reg, "gx wipe", req(t, `{"flags":{"yes":true}}`), Options{Dir: e.dir})
	if errorOf(t, res)["code"] != "E425" {
		t.Errorf("without a Guard: %s", res.Stdout)
	}
	// E425 is audited as a refusal, with the flag value kept out of the line.
	lines := e.auditLines()
	if len(lines) != len(cases) || lines[0]["decision"] != "refused" || lines[0]["error_code"] != "E425" || lines[0]["exit_code"] != nil {
		t.Errorf("audit: %v", lines)
	}
	// a flag that is not invoker-owned stays an ordinary E420.
	if x := errorOf(t, e.call("gx put", `{"args":["k","v"],"flags":{"plan":true}}`)); x["code"] != "E420" {
		t.Errorf("unknown flag on a command without a confirm block: %v", x)
	}
}

// TestGateAuditProbe pins the fail-closed audit rule for gated runs: a
// confirmed destructive call whose audit log cannot be written is refused
// (E421) and runs nothing, a Guard with no audit path refuses the same way,
// and a bad request never gets a nonce (validated before issue).
func TestGateAuditProbe(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"unwritable": filepath.Join(blocker, "logs", "machine-audit.jsonl"), "no path": ""} {
		e := newGateEnv(t)
		e.guard = NewGuard(path)
		conf := confirmationOf(t, e.call("gx wipe", `{"args":["n1"],"flags":{"target":"t"}}`))
		x := errorOf(t, e.call("gx wipe", `{"args":["n1"],"flags":{"target":"t"},"confirm":"`+conf.Confirm+`"}`))
		if x["code"] != "E421" || !strings.Contains(x["cause"].(string), "audit") {
			t.Errorf("%s: want E421 naming the audit log, got %v", name, x)
		}
		if len(e.argvs()) != 0 {
			t.Errorf("%s: the command ran: %v", name, e.argvs())
		}
	}
	e := newGateEnv(t)
	x := errorOf(t, e.call("gx wipe", `{"args":["n1"],"flags":{"target":"t","nope":1}}`))
	if x["code"] != "E420" || e.guard.liveNonces() != 0 {
		t.Errorf("a bad request must be E420 with no nonce issued: %v live=%d", x, e.guard.liveNonces())
	}
}
