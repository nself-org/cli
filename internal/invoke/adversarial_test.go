package invoke

// Purpose: TestInvokeAdversarial, the P7-SURF-21 adversarial suite of the
// invoker and its confirmation gate. Each theme file registers its cases as
// subtests of this one test.
// Inputs: the gate and fixture registries and the stub child of this package.
// Constraints: every subtest asserts the SAFE outcome (a refusal with the
// right E-code and no child run, or a hostile value that reaches the child as
// exactly one argv element). Fake secrets are obviously synthetic. No sleeps:
// time is the Guard's injected clock.

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestInvokeAdversarial is the single entry point; the themes are in the
// adversarial_*_test.go files.
func TestInvokeAdversarial(t *testing.T) {
	t.Run("argv-flag-smuggling", advArgvSmuggling)
	t.Run("owned-flags", advOwnedFlags)
	t.Run("control-chars-and-paths", advControlChars)
	t.Run("size-limits", advSizeLimits)
	t.Run("deep-nesting", advDeepNesting)
	t.Run("duplicate-and-folded-members", advMembers)
	t.Run("confirm-replay-and-reuse", advConfirmReplay)
	t.Run("confirm-expiry-and-flood", advConfirmExpiryFlood)
	t.Run("confirm-string-tricks", advConfirmStrings)
	t.Run("consent-flag-injection", advConsentInjection)
	t.Run("plan-form-confirm", advPlanForm)
	t.Run("prompt-injection-and-shell-metachars", advInjection)
	t.Run("audit-and-token-leaks", advLeaks)
	t.Run("audit-line-validity", advAuditValidity)
	t.Run("concurrent-destructive", advConcurrent)
}

// advGate is the gate fixture (gx commands) with the stub in echo mode.
func advGate(t *testing.T, kv ...string) *gateEnv { return newGateEnv(t, kv...) }

// advFixture is the fixture registry (fx echo, plugin commands) with the same
// stub, Guard and logs as advGate.
func advFixture(t *testing.T, kv ...string) *gateEnv {
	t.Helper()
	e := newGateEnv(t, kv...)
	e.reg = fixtureRegistry(t)
	return e
}

// doc renders v as a request document.
func doc(t testing.TB, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// echoedArgv is the argv the stub child (echo mode) reports it received.
func echoedArgv(t *testing.T, res Result) []string {
	t.Helper()
	if res.ErrorCode != "" || res.ExitCode != 0 {
		t.Fatalf("the call was refused: code=%q exit=%d %s", res.ErrorCode, res.ExitCode, res.Stdout)
	}
	d := decodeDoc(t, res.Stdout)
	data, ok := d["data"].(map[string]any)
	if !ok {
		t.Fatalf("no data in %s", res.Stdout)
	}
	raw, _ := data["argv"].([]any)
	out := make([]string, len(raw))
	for i, v := range raw {
		out[i], _ = v.(string)
	}
	return out
}

// wantRefusal asserts res is the error envelope of one of codes and that the
// stub child was started exactly ran times in total.
func (e *gateEnv) wantRefusal(res Result, ran int, codes ...string) {
	e.t.Helper()
	got, _ := errorOf(e.t, res)["code"].(string)
	ok := false
	for _, c := range codes {
		ok = ok || got == c
	}
	if !ok || res.ErrorCode != got {
		e.t.Errorf("want one of %v, got %q (result code %q): %s", codes, got, res.ErrorCode, res.Stdout)
	}
	if n := len(e.argvs()); n != ran {
		e.t.Errorf("the child ran %d times, want %d: %v", n, ran, e.argvs())
	}
}

// repeatJSON builds open*n + inner + close*n.
func repeatJSON(open, inner, closer string, n int) string {
	return strings.Repeat(open, n) + inner + strings.Repeat(closer, n)
}
