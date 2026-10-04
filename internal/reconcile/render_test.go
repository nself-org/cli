package reconcile

import (
	"bytes"
	"testing"
)

func render(t *testing.T, p Plan) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := RenderHuman(&buf, p); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRenderHumanGoldens(t *testing.T) {
	golden(t, "render-full.txt", render(t, fullPlan(t)))
	golden(t, "render-empty.txt", render(t, emptyPlan(t, "dev")))
}

func TestRenderHumanIsDeterministic(t *testing.T) {
	p := fullPlan(t)
	first := render(t, p)
	for i := 0; i < 20; i++ {
		if !bytes.Equal(first, render(t, p)) {
			t.Fatal("render differs between calls")
		}
	}
}

func TestRenderHeaderUsesTwelveCharPrefix(t *testing.T) {
	p := fullPlan(t)
	want := "Plan " + p.PlanID[:12] + " (build, env prod/prod)\n"
	if got := string(render(t, p)); len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("header = %q, want prefix %q", got, want)
	}
}
