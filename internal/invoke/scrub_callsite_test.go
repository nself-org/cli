package invoke

// Purpose: pin the scrubSet call sites in Invoke and planGate (D-0468).
// Inputs: the fixture registries and the stub child. Outputs: pass/fail.
// Constraints: values are obviously fake; each test has a positive control.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
)

// scrubCallSiteStderr is what the stub child writes: the rendered secret 7
// (sent as 007) and a non-secret marker that proves the cause is not empty.
const scrubCallSiteStderr = "boom code 7 here marker-keep-0468"

// assertScrubbedCause checks a refusal cause hides the rendered value and
// still carries the marker (positive control).
func assertScrubbedCause(t *testing.T, res Result) {
	t.Helper()
	e := errorOf(t, res)
	c, _ := e["cause"].(string)
	if e["code"] != "E422" || !strings.Contains(c, "marker-keep-0468") {
		t.Fatalf("positive control: want E422 with the marker, got %v", e)
	}
	if strings.Contains(c, "code 7") || !strings.Contains(c, Redacted) {
		t.Errorf("the rendered secret reached the cause: %q", c)
	}
}

// TestInvokeScrubsRenderedSecretFromRefusal pins the scrubSet call site in
// Invoke (invoke.go): reverting it to requestSecrets alone fails this test.
func TestInvokeScrubsRenderedSecretFromRefusal(t *testing.T) {
	reg := fixtureRegistry(t)
	cmd := mustCmd(t, reg, "fx echo") // the registry is built per test, so editing it is safe
	cmd.Flags = append([]cmdregistry.Flag{}, cmd.Flags...)
	for i := range cmd.Flags {
		if cmd.Flags[i].Name == "count" {
			cmd.Flags[i].Secret = true
		}
	}
	useStub(t, "STUBCLI_MODE", "text", "STUBCLI_STDERR", scrubCallSiteStderr)
	r := Request{Args: []string{"w"}, Flags: map[string]any{"count": json.Number("007")}}
	res, err := Invoke(context.Background(), reg, "fx echo", r, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertScrubbedCause(t, res)
}

// TestPlanGateScrubsRenderedSecretFromRefusal pins the scrubSet call site in
// planGate (gate.go): the plan exec's refusal must not carry the rendered 7.
func TestPlanGateScrubsRenderedSecretFromRefusal(t *testing.T) {
	cmd := *mustCmd(t, gateRegistry(t), "gx apply")
	cmd.Flags = append(append([]cmdregistry.Flag{}, cmd.Flags...), cmdregistry.Flag{Name: "count", Type: "int", Secret: true})
	useStub(t, "STUBCLI_MODE", "text", "STUBCLI_STDERR", scrubCallSiteStderr)
	r := Request{Flags: map[string]any{"count": json.Number("007")}}
	d, err := planGate(context.Background(), &cmd, r, Options{}, Decision{SideEffect: "write"}, false)
	if err != nil || d.Result == nil {
		t.Fatalf("want a refused plan exec, got %+v %v", d, err)
	}
	assertScrubbedCause(t, *d.Result)
}
