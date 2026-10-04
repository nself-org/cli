package reconcile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// contractKeys are the change-plan v1 field orders from EPIC §Contracts.
var contractKeys = map[string][]string{
	"plan": {"schema_version", "plan_id", "command", "trigger", "env", "env_class", "artifacts", "effects",
		"containers", "destructive", "destructive_reasons", "requires_confirmation", "empty"},
	"trigger":    {"kind", "subject"},
	"artifacts":  {"kind", "path", "action", "generated", "hand_edited", "diff_lines", "redacted"},
	"effects":    {"kind", "target", "detail"},
	"containers": {"known", "items"},
	"items":      {"service", "action", "stateful", "applied_by"},
}

// objectKeys returns the keys of the first JSON object found at raw, in order.
func objectKeys(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("not an object: %s", raw)
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func TestPlanJSONFieldOrderMatchesContract(t *testing.T) {
	b, err := MarshalPlan(fullPlan(t))
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatal(err)
	}
	check := func(label string, raw json.RawMessage) {
		if got := objectKeys(t, raw); !reflect.DeepEqual(got, contractKeys[label]) {
			t.Errorf("%s keys = %v, want %v", label, got, contractKeys[label])
		}
	}
	check("plan", b)
	check("trigger", top["trigger"])
	check("containers", top["containers"])
	var arts, effs []json.RawMessage
	var cont struct{ Items []json.RawMessage }
	_ = json.Unmarshal(top["artifacts"], &arts)
	_ = json.Unmarshal(top["effects"], &effs)
	_ = json.Unmarshal(top["containers"], &cont)
	check("artifacts", arts[0])
	check("effects", effs[0])
	check("items", cont.Items[0])
}

func TestPlanGoldens(t *testing.T) {
	for name, p := range map[string]Plan{
		"plan-full.json":       fullPlan(t),
		"plan-empty-prod.json": emptyPlan(t, "prod"),
		"plan-empty-dev.json":  emptyPlan(t, ""),
	} {
		b, err := MarshalPlan(p)
		if err != nil {
			t.Fatal(err)
		}
		golden(t, name, b)
	}
}

func TestEmptyPlanFlags(t *testing.T) {
	for _, env := range []string{"prod", "production", "staging", "dev", "", "local"} {
		p := emptyPlan(t, env)
		if !p.Empty || p.RequiresConfirmation || p.Destructive {
			t.Errorf("env %q: empty=%v requires_confirmation=%v destructive=%v", env, p.Empty, p.RequiresConfirmation, p.Destructive)
		}
		if !strings.Contains(mustJSON(t, p), `"artifacts": []`) {
			t.Errorf("env %q: empty arrays must render as []", env)
		}
	}
}

func mustJSON(t *testing.T, p Plan) string {
	t.Helper()
	b, err := MarshalPlan(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFinalizeSortsAndDropsNone(t *testing.T) {
	p := fullPlan(t)
	paths := []string{p.Artifacts[0].Path, p.Artifacts[1].Path, p.Artifacts[2].Path, p.Artifacts[3].Path}
	want := []string{".env.computed", "docker-compose.yml", "nginx/sites/api.conf", "nself.lock.yaml"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("artifact order = %v", paths)
	}
	if p.Effects[0].Kind != EffectHosts || p.Effects[1].Kind != EffectOrphanRemove {
		t.Errorf("effects not sorted by kind: %+v", p.Effects)
	}
	if len(p.Containers.Items) != 2 || p.Containers.Items[0].Service != "nginx" {
		t.Errorf("containers = %+v", p.Containers.Items)
	}
	again := p
	if err := again.Finalize(); err != nil || again.PlanID != p.PlanID {
		t.Errorf("Finalize not idempotent: %v %s vs %s", err, again.PlanID, p.PlanID)
	}
}

// randomPlan builds a finalised plan from rng.
func randomPlan(t *testing.T, rng *rand.Rand) Plan {
	t.Helper()
	pick := func(xs ...string) string { return xs[rng.Intn(len(xs))] }
	p := Plan{
		Command: Command(pick("build", "config-set", "doctor-fix", "plugin-install")),
		Trigger: Trigger{Kind: TriggerKind(pick("build", "config", "plugin", "drift")), Subject: pick("", "KEY", "ai")},
		Env:     pick("prod", "dev", "staging", "local", ""),
	}
	for i := rng.Intn(5); i > 0; i-- {
		p.Artifacts = append(p.Artifacts, Artifact{
			Kind: KindOther, Path: pick("a", "b/c", "d.conf", ".env") + string(rune('0'+rng.Intn(9))),
			Action: Action(pick("add", "change", "remove")), Generated: rng.Intn(2) == 0,
			HandEdited: rng.Intn(2) == 0, DiffLines: rng.Intn(50) - 1, Redacted: rng.Intn(2) == 0,
		})
	}
	for i := rng.Intn(4); i > 0; i-- {
		p.Effects = append(p.Effects, Effect{Kind: EffectKind(pick("hosts", "orphan-remove", "plugin-remove")),
			Target: pick("x", "y", "z") + string(rune('0'+rng.Intn(9))), Detail: pick("", "d1", "d2")})
	}
	p.Containers.Known = rng.Intn(2) == 0
	for i := rng.Intn(4); i > 0; i-- {
		p.Containers.Items = append(p.Containers.Items, ContainerItem{
			Service: pick("db", "web", "redis") + string(rune('0'+rng.Intn(9))), Action: ContainerRecreate,
			Stateful: rng.Intn(2) == 0, AppliedBy: AppliedBy(pick("next-start", "this-command"))})
	}
	if err := p.Finalize(); err != nil {
		t.Fatal(err)
	}
	return p
}

// mutate returns a copy of p with one item changed, or false when p has none.
func mutate(p Plan, rng *rand.Rand) (Plan, bool) {
	q := p
	q.Artifacts = append([]Artifact(nil), p.Artifacts...)
	q.Effects = append([]Effect(nil), p.Effects...)
	q.Containers.Items = append([]ContainerItem(nil), p.Containers.Items...)
	var opts []func()
	if n := len(q.Artifacts); n > 0 {
		i := rng.Intn(n)
		opts = append(opts, func() { q.Artifacts[i].Path += "x" }, func() { q.Artifacts[i].DiffLines += 100 })
	}
	if n := len(q.Effects); n > 0 {
		i := rng.Intn(n)
		opts = append(opts, func() { q.Effects[i].Detail += "x" }, func() { q.Effects[i].Target += "x" })
	}
	if n := len(q.Containers.Items); n > 0 {
		i := rng.Intn(n)
		opts = append(opts, func() { q.Containers.Items[i].Service += "x" }, func() { q.Containers.Items[i].Stateful = !q.Containers.Items[i].Stateful })
	}
	if len(opts) == 0 {
		return q, false
	}
	opts[rng.Intn(len(opts))]()
	return q, true
}

func TestPlanIDProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for n := 0; n < 500; n++ {
		p := randomPlan(t, rng)
		canon, err := CanonicalJSON(p)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(canon)
		if p.PlanID != hex.EncodeToString(sum[:]) || len(p.PlanID) != 64 {
			t.Fatalf("plan %d: plan_id %q != sha256(canonical)", n, p.PlanID)
		}
		blank := p
		blank.PlanID = ""
		if again, _ := PlanID(blank); again != p.PlanID {
			t.Fatalf("plan %d: recomputation differs", n)
		}
		if q, ok := mutate(p, rng); ok {
			if id, _ := PlanID(q); id == p.PlanID {
				t.Fatalf("plan %d: changing an item kept plan_id %s", n, id)
			}
		}
	}
}

func TestCanonicalJSONIgnoresInputPlanIDAndNil(t *testing.T) {
	p := fullPlan(t)
	q := p
	q.PlanID = "deadbeef"
	a, _ := CanonicalJSON(p)
	b, _ := CanonicalJSON(q)
	if !bytes.Equal(a, b) {
		t.Error("canonical JSON must not include plan_id")
	}
	if p.PlanID == "" || strings.Contains(string(a), p.PlanID) {
		t.Error("plan_id must be set on the plan but absent from its canonical form")
	}
	var zero Plan
	if c, _ := CanonicalJSON(zero); strings.Contains(string(c), "null") {
		t.Errorf("nil slices must encode as []: %s", c)
	}
}
