package invoke

// Purpose: adversarial audit-log, token-leak and concurrency tests of
// TestInvokeAdversarial.
// Constraints: secrets are obviously synthetic (nself_test_FAKE_*); the
// stub child echoes them to stderr to model a child that leaks. Goroutines call
// Invoke directly: t.Fatal is not allowed off the test goroutine.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
)

// Synthetic secrets. Only S3 and S5 are shaped like tokens the generic
// redactor knows; S1 and S2 are protected by the registry (a secret arg, a
// secret-named flag) and must never appear anywhere.
const (
	fakeS1 = "nself_test_FAKE_token_0001"
	fakeS2 = "nself_test_FAKE_pw_0002"
	fakeS3 = "ghp_FAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKE0003"
	fakeS4 = "plain-secret-in-a-third-positional-0004"
	fakeS5 = "sk-FAKEFAKEFAKEFAKEFAKE0005"
)

// auditText is the whole audit file (with rotated siblings absent in tests).
func (e *gateEnv) auditText() string {
	e.t.Helper()
	b, err := os.ReadFile(e.audit)
	if os.IsNotExist(err) {
		return ""
	} else if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

func advLeaks(t *testing.T) {
	leaky := "child says: " + fakeS1 + " " + fakeS2 + " " + fakeS3 + " " + fakeS5
	t.Run("secrets in args, flags and secret_args never reach the log or a response", func(t *testing.T) {
		e := advGate(t, "STUBCLI_MODE", "text", "STUBCLI_STDERR", leaky, "STUBCLI_EXIT", "1")
		var seen strings.Builder
		run := func(path, body string) Result {
			res := e.call(path, body)
			seen.Write(res.Stdout)
			seen.Write(res.Stderr)
			if len(res.Stderr) != 0 {
				t.Errorf("%s: Invoke must clear the raw stderr", path)
			}
			return res
		}
		run("gx put", doc(t, map[string]any{"args": []string{fakeS5, fakeS1}, "flags": map[string]any{"db-password": fakeS2, "note": fakeS3}}))
		run("gx put", doc(t, map[string]any{"args": []string{"k", fakeS1, fakeS4}}))                                               // a third, undeclared positional
		run("gx put", doc(t, map[string]any{"args": []string{"k", "v"}, "flags": map[string]any{"undeclared-" + fakeS3: fakeS1}})) // a token-shaped unknown flag name
		unknown := e.call(fakeS5+" "+fakeS3, doc(t, map[string]any{"args": []string{fakeS1}}))
		// The envelope's top-level "command" echoes the caller's own path (finding A3);
		// every other member of it must be clean.
		var env map[string]any
		_ = json.Unmarshal(unknown.Stdout, &env)
		delete(env, "command")
		rest, _ := json.Marshal(env)
		seen.Write(rest) // an unknown command
		run("gx wipe", doc(t, map[string]any{"flags": map[string]any{"target": fakeS3}}))
		run("gx put", doc(t, map[string]any{"args": []string{"k", fakeS1}, "flags": map[string]any{"db-password": []string{fakeS2, fakeS4}}}))

		audit := e.auditText()
		if audit == "" {
			t.Fatal("no audit lines were written")
		}
		for name, s := range map[string]string{"S1": fakeS1, "S2": fakeS2, "S3": fakeS3, "S4": fakeS4, "S5": fakeS5} {
			if strings.Contains(audit, s) {
				t.Errorf("%s (%s) is in the audit log", name, s)
			}
			if strings.Contains(seen.String(), s) {
				t.Errorf("%s (%s) is in a returned envelope", name, s)
			}
		}
		// the registry secrets never sat on the child argv either
		raw, _ := os.ReadFile(e.argvLog)
		for name, s := range map[string]string{"S1": fakeS1, "S2": fakeS2} {
			if bytes.Contains(raw, []byte(s)) {
				t.Errorf("%s reached the child argv: %s", name, raw)
			}
		}
	})

	t.Run("a failing read of free-form argv does not log or return the argv text", func(t *testing.T) {
		e := advFixture(t, "STUBCLI_MODE", "text", "STUBCLI_STDERR", leaky, "STUBCLI_EXIT", "1")
		res := e.call("demo", doc(t, map[string]any{"argv": []string{"--token", fakeS1, fakeS2}}))
		if strings.Contains(string(res.Stdout), fakeS1) || strings.Contains(string(res.Stdout), fakeS2) {
			t.Errorf("a free-form argv secret came back: %s", res.Stdout)
		}
		if a := e.auditText(); strings.Contains(a, fakeS1) || strings.Contains(a, fakeS2) {
			t.Errorf("a free-form argv secret was logged: %s", a)
		}
	})

	t.Run("a child that prints a registry secret in stderr gets it scrubbed in every spelling", func(t *testing.T) {
		quoted := `"` + fakeS1 + `" \"` + fakeS1 + `\" ` + `{"v":"` + fakeS1 + `"}`
		e := advGate(t, "STUBCLI_MODE", "text", "STUBCLI_STDERR", quoted+" "+fakeS2, "STUBCLI_EXIT", "1")
		res := e.call("gx put", doc(t, map[string]any{"args": []string{"k", fakeS1}, "flags": map[string]any{"db-password": fakeS2}}))
		if strings.Contains(string(res.Stdout), fakeS1) || strings.Contains(string(res.Stdout), fakeS2) {
			t.Errorf("stderr secret survived scrubbing: %s", res.Stdout)
		}
		if errorOf(t, res)["code"] != "E422" {
			t.Errorf("want E422: %s", res.Stdout)
		}
	})

	t.Run("a confirmation nonce is never logged, echoed in an error or kept in the audit", func(t *testing.T) {
		e := advGate(t)
		n := nonceFor(t, e, "gx wipe", map[string]any{"flags": map[string]any{"target": "a"}})
		spent := e.call("gx wipe", withConfirm(t, map[string]any{"flags": map[string]any{"target": "a"}}, n))
		replay := e.call("gx wipe", withConfirm(t, map[string]any{"flags": map[string]any{"target": "a"}}, n))
		mismatch := e.call("gx push", withConfirm(t, map[string]any{}, n))
		for _, r := range []Result{spent, replay, mismatch} {
			if bytes.Contains(r.Stdout, []byte(n)) {
				t.Errorf("a response echoes the nonce: %s", r.Stdout)
			}
		}
		if a := e.auditText(); strings.Contains(a, n) || strings.Contains(a, shaHex(n)) {
			t.Errorf("the audit log holds the nonce or its key:\n%s", a)
		}
		// the request id is a name for the request: it must not be the nonce or a hash of it
		for _, l := range e.auditLines() {
			if id, _ := l["request_id"].(string); id == n || id == shaHex(n) {
				t.Errorf("request_id equals the nonce or its digest")
			}
		}
	})

	t.Run("the request id does not depend on a secret value", func(t *testing.T) {
		e := advGate(t)
		a := e.call("gx put", doc(t, map[string]any{"args": []string{"k", fakeS1}}))
		b := e.call("gx put", doc(t, map[string]any{"args": []string{"k", fakeS4}}))
		if a.RequestID == "" || a.RequestID != b.RequestID {
			t.Errorf("request ids differ by secret value: %q vs %q", a.RequestID, b.RequestID)
		}
	})
}

// advAuditValidity: every audit line is one compact JSON object, however
// hostile the request text.
func advAuditValidity(t *testing.T) {
	hostile := []string{
		"line1\nline2", "cr\rlf", "tab\there", "\x1b[31mred\x1b[0m", "bell\a", "\u2028sep\u2029", "\x7f", "\u0085next",
		"quote\"back\\slash", "}\n{\"decision\":\"run\"}", "{\"time\":\"x\"}\n", "\ufeffbom", "\xff\xfe", "nul\x00byte",
	}
	t.Run("hostile text cannot forge, split or break an audit line", func(t *testing.T) {
		e := advGate(t)
		calls := 0
		for _, h := range hostile {
			e.call("gx put", doc(t, map[string]any{"args": []string{h, "v"}, "flags": map[string]any{"note": h}}))
			e.call("gx put", doc(t, map[string]any{"args": []string{"k", "v"}, "flags": map[string]any{h: "x"}}))
			e.call("gx wipe", doc(t, map[string]any{"args": []string{h}}))
			e.call("zz "+h, `{}`)
			calls += 4
		}
		raw := e.auditText()
		lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
		if len(lines) != calls {
			t.Fatalf("%d audited calls produced %d lines", calls, len(lines))
		}
		for i, l := range lines {
			var m map[string]any
			if err := json.Unmarshal([]byte(l), &m); err != nil {
				t.Fatalf("line %d is not JSON (%v): %q", i, err, l)
			}
			if m["time"] == "x" || len(m) != 12 {
				t.Errorf("line %d has %d members, want the 12 of the contract: %q", i, len(m), l)
			}
			for _, bad := range []string{"\r", "\x1b", "\u2028", "\u2029", "\u0085", "\x00", "\a", "\t"} {
				if strings.Contains(l, bad) {
					t.Errorf("line %d holds the raw character %q", i, bad)
				}
			}
			if id, _ := m["request_id"].(string); !confirmPattern.MatchString(id) {
				t.Errorf("line %d request_id %q", i, id)
			}
		}
		if !strings.HasSuffix(raw, "\n") {
			t.Error("the audit file does not end with a newline")
		}
	})
}

// advConcurrent: the nonce is spent exactly once under a race, one Guard.
func advConcurrent(t *testing.T) {
	const workers = 20
	invokeAll := func(e *gateEnv, bodies []string) []Result {
		out := make([]Result, len(bodies))
		errsOut := make([]error, len(bodies))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range bodies {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				r, err := DecodeRequest([]byte(bodies[i]))
				if err != nil {
					errsOut[i] = err
					return
				}
				out[i], errsOut[i] = Invoke(context.Background(), e.reg, "gx wipe", r, Options{Dir: e.dir, Guard: e.guard})
			}(i)
		}
		close(start)
		wg.Wait()
		for _, err := range errsOut {
			if err != nil {
				t.Fatalf("a concurrent call failed: %v", err)
			}
		}
		return out
	}

	t.Run("one nonce presented by 20 goroutines runs the child exactly once", func(t *testing.T) {
		e := advGate(t)
		for round := 0; round < 3; round++ {
			m := map[string]any{"flags": map[string]any{"target": "r" + string(rune('0'+round))}}
			n := nonceFor(t, e, "gx wipe", m)
			bodies := make([]string, workers)
			for i := range bodies {
				bodies[i] = withConfirm(t, m, n)
			}
			ok, denied := 0, 0
			for _, r := range invokeAll(e, bodies) {
				switch r.ErrorCode {
				case "":
					ok++
				case "E424":
					denied++
				default:
					t.Errorf("unexpected result %q: %s", r.ErrorCode, r.Stdout)
				}
			}
			if ok != 1 || denied != workers-1 {
				t.Errorf("round %d: %d runs and %d refusals, want 1 and %d", round, ok, denied, workers-1)
			}
			if got := len(e.argvs()); got != round+1 {
				t.Errorf("round %d: the child ran %d times in total, want %d", round, got, round+1)
			}
		}
	})

	t.Run("20 different nonces through one Guard each run once, audit stays whole", func(t *testing.T) {
		e := advGate(t)
		bodies := make([]string, workers)
		for i := range bodies {
			m := map[string]any{"flags": map[string]any{"target": "d" + string(rune('a'+i))}}
			bodies[i] = withConfirm(t, m, nonceFor(t, e, "gx wipe", m))
		}
		for i, r := range invokeAll(e, bodies) {
			if r.ErrorCode != "" || r.ExitCode != 0 {
				t.Errorf("call %d: %q %s", i, r.ErrorCode, r.Stdout)
			}
		}
		if got := len(e.argvs()); got != workers {
			t.Errorf("the child ran %d times, want %d", got, workers)
		}
		runs := 0
		for _, l := range e.auditLines() { // auditLines fails the test on a torn or invalid line
			if l["decision"] == "run" {
				runs++
			}
		}
		if runs != workers {
			t.Errorf("%d run lines in the audit log, want %d", runs, workers)
		}
	})

	t.Run("concurrent issue of one request yields distinct live nonces", func(t *testing.T) {
		e := advGate(t)
		bodies := make([]string, workers)
		for i := range bodies {
			bodies[i] = `{"flags":{"target":"same"}}`
		}
		seen := map[string]bool{}
		for _, r := range invokeAll(e, bodies) {
			seen[confirmationOf(t, r).Confirm] = true
		}
		if len(seen) != workers || e.guard.liveNonces() != workers {
			t.Errorf("%d distinct nonces, %d live, want %d", len(seen), e.guard.liveNonces(), workers)
		}
		if len(e.argvs()) != 0 {
			t.Error("issuing a nonce started a child")
		}
	})

	t.Run("a nonce raced against its own expiry is spent at most once", func(t *testing.T) {
		e := advGate(t)
		m := map[string]any{"flags": map[string]any{"target": "x"}}
		n := nonceFor(t, e, "gx wipe", m)
		e.clock.advance(nonceTTL - 1)
		bodies := make([]string, workers)
		for i := range bodies {
			bodies[i] = withConfirm(t, m, n)
		}
		go e.clock.advance(1) // whichever side of the TTL a caller lands, no more than one run
		ok := 0
		for _, r := range invokeAll(e, bodies) {
			if r.ErrorCode == "" {
				ok++
			}
		}
		if ok > 1 || len(e.argvs()) != ok {
			t.Errorf("%d successful spends, %d child runs", ok, len(e.argvs()))
		}
	})
}
