package plan

// plan_test.go — table-driven tests for Diff, DefaultSteps, ApplyImagePins and
// Render. No network and no disk.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/lock"
)

const (
	dOld = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	dNew = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func fixtureLock() lock.Lock {
	l := lock.New()
	l.CLI = "1.4"
	l.Images = map[string]string{"redis": "docker.io/library/redis:7@" + dOld, "nginx": "docker.io/library/nginx:1@" + dOld}
	l.Unpinned = map[string]string{"minio": "docker.io/pgsty/minio:latest"}
	l.Plugins = map[string]lock.PluginPin{"ai": {Version: "1.0.0", Tier: lock.TierFree}, "cron": {Version: "2.0.0", Tier: lock.TierFree}}
	return l
}

func TestDiff(t *testing.T) {
	cands := Candidates{
		CLICurrent: "v1.4.2",
		CLI:        Candidate{Value: "v1.4.9"},
		Images: map[string]Candidate{
			"redis": {Value: "docker.io/library/redis:8@" + dNew},
			"nginx": {Value: "nginx:1@" + dOld}, // same digest, other spelling: no row
			"minio": {Reason: "registry answered 404"},
		},
		Plugins: map[string]Candidate{"cron": {Value: "2.0.0"}}, // ai is missing
	}
	got := Diff(fixtureLock(), cands)
	type row struct {
		kind, name string
		ok         bool
	}
	want := []row{{"cli", "nself", true}, {"image", "minio", false}, {"image", "redis", true}, {"plugin", "ai", false}}
	if len(got) != len(want) {
		t.Fatalf("rows = %+v, want %d", got, len(want))
	}
	for i, w := range want {
		if got[i].Kind != w.kind || got[i].Name != w.name || got[i].Resolvable != w.ok {
			t.Errorf("row %d = %+v, want %+v", i, got[i], w)
		}
		if !w.ok && got[i].Reason == "" {
			t.Errorf("row %d unresolvable without a reason: %+v", i, got[i])
		}
		if w.ok && got[i].To == "" {
			t.Errorf("row %d resolvable without a target: %+v", i, got[i])
		}
	}
	if got[3].Reason != ReasonNoPlugin {
		t.Errorf("missing plugin reason = %q", got[3].Reason)
	}
}

func TestDiffEmptyWhenCurrent(t *testing.T) {
	l := fixtureLock()
	c := Candidates{CLICurrent: "1.4.2", CLI: Candidate{Value: "v1.4.2"},
		Images:  map[string]Candidate{"redis": {Value: l.Images["redis"]}, "nginx": {Value: l.Images["nginx"]}, "minio": {Value: "docker.io/pgsty/minio:latest"}},
		Plugins: map[string]Candidate{"ai": {Value: "1.0.0"}, "cron": {Value: "2.0.0"}}}
	if got := Diff(l, c); len(got) != 0 {
		t.Fatalf("rows = %+v, want none", got)
	}
}

func TestDiffCLIUnresolvable(t *testing.T) {
	got := Diff(lock.New(), Candidates{CLICurrent: "1.4.2", CLI: Candidate{Reason: "no release found (HTTP 404)"}})
	if len(got) != 1 || got[0].Resolvable || got[0].Kind != KindCLI || got[0].Reason == "" {
		t.Fatalf("rows = %+v", got)
	}
}

func TestDefaultStepsOrder(t *testing.T) {
	pins := []Pin{{Kind: KindImage, Name: "redis", Resolvable: true}, {Kind: KindImage, Name: "gone"}, {Kind: KindPlugin, Name: "ai", Resolvable: true}, {Kind: KindCLI, Name: "nself", Resolvable: true}}
	steps := DefaultSteps(pins)
	var names []string
	for _, s := range steps {
		names = append(names, s.Name)
		if s.Pins == nil {
			t.Errorf("step %s has nil Pins (JSON would print null)", s.Name)
		}
	}
	if got, want := strings.Join(names, ","), "backup,images,plugins,build,start,migrate,readiness,health,cli"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	if len(steps[1].Pins) != 1 || len(steps[2].Pins) != 1 || len(steps[8].Pins) != 1 {
		t.Fatalf("pins not attached to their steps (unresolvable ones must not be): %+v", steps)
	}
}

func TestApplyImagePins(t *testing.T) {
	l := fixtureLock()
	newRedis := "docker.io/library/redis:8@" + dNew
	newMinio := "docker.io/pgsty/minio:2@" + dNew
	out := ApplyImagePins(l, []Pin{
		{Kind: KindImage, Name: "redis", To: newRedis, Resolvable: true},
		{Kind: KindImage, Name: "minio", To: newMinio, Resolvable: true},
		{Kind: KindImage, Name: "nginx", Resolvable: false}, // unresolvable: untouched
		{Kind: KindImage, Name: "bad", To: "no-digest:1", Resolvable: true},
	})
	if out.Images["redis"] != newRedis || out.Images["minio"] != newMinio {
		t.Fatalf("images = %v", out.Images)
	}
	if _, still := out.Unpinned["minio"]; still {
		t.Fatal("minio is both pinned and unpinned")
	}
	if out.Images["nginx"] != l.Images["nginx"] || out.Images["bad"] != "" {
		t.Fatalf("untouched pins changed: %v", out.Images)
	}
	if l.Images["redis"] == newRedis {
		t.Fatal("ApplyImagePins mutated its input")
	}
	if _, err := out.Marshal(); err != nil {
		t.Fatalf("result is not a valid lock: %v", err)
	}
}

func TestRender(t *testing.T) {
	var b bytes.Buffer
	p := BuildPlan(fixtureLock(), Candidates{CLICurrent: "1.4.2", CLI: Candidate{Value: "v1.4.9"},
		Images: map[string]Candidate{"redis": {Value: "docker.io/library/redis:8@" + dNew}}})
	if err := Render(&b, p); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, w := range []string{"KIND", "redis", "unresolvable", ReasonNoImage, "Steps:", "1. backup", "9. cli"} {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
	b.Reset()
	if err := Render(&b, Plan{Pins: []Pin{}, Steps: DefaultSteps(nil)}); err != nil || !strings.Contains(b.String(), "No pin changes") {
		t.Fatalf("empty plan: %v %q", err, b.String())
	}
}
