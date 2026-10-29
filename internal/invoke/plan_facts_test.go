package invoke

import (
	"strings"
	"testing"
)

// TestGatePlanDeclaringSecretsConfirms (P7-SHIP-02 gap 2): a write plan that
// needs no confirmation by its own rules but declares the secrets-persist step
// is returned as a kind plan confirmation, and only the plan exec ran.
func TestGatePlanDeclaringSecretsConfirms(t *testing.T) {
	id := shaHex("first build")
	e := newGateEnv(t, "STUBCLI_MODE", "raw", "STUBCLI_STDOUT",
		`{"schema_version":"1","command":"gx apply","data":{"plan_id":"`+id+`","requires_confirmation":false,`+
			`"effects":[{"kind":"secrets-persist","target":".env.secrets","detail":""}]}}`)
	conf := confirmationOf(t, e.call("gx apply", `{"flags":{"target":"x"}}`))
	if conf.Kind != ConfirmKindPlan || conf.Confirm != id || conf.SideEffect != "write" {
		t.Fatalf("confirmation: %+v", conf)
	}
	if av := e.argvs(); len(av) != 1 || !hasArg(av[0], "--plan=true") {
		t.Fatalf("only the plan exec runs: %q", av)
	}
}

// TestParsePlanSecrets: the secrets fact comes from data.effects only.
func TestParsePlanSecrets(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, tc := range []struct {
		effects string
		want    bool
	}{
		{`[]`, false},
		{`[{"kind":"hosts"}]`, false},
		{`[{"kind":"hosts"},{"kind":"secrets-persist"}]`, true},
	} {
		doc := `{"data":{"plan_id":"` + id + `","requires_confirmation":false,"effects":` + tc.effects + `}}`
		f, childErr, problem := parsePlan([]byte(doc))
		if childErr || problem != "" || f.secrets != tc.want || f.id != id {
			t.Errorf("%s: %+v %v %q", tc.effects, f, childErr, problem)
		}
	}
}
