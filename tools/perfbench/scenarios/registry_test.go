package scenarios

// Purpose: pin the registry contract and every scenario's probe definitions, so
// a changed metric name, argument list or expected exit code fails here instead
// of silently moving a baseline (D-0234).
// Constraints: no process, network or filesystem work.

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestRegistryNames(t *testing.T) {
	want := []string{"cold-start", "cold-start-plugins", "cold-start-plugins-v15", "http-invoke", "json-overhead"}
	got := Names()
	if !sort.StringsAreSorted(got) {
		t.Fatalf("Names() not sorted: %v", got)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for _, n := range want {
		s, ok := Get(n)
		if !ok || s.Name() != n {
			t.Errorf("Get(%q) = %v, %v", n, s, ok)
		}
	}
	if s, ok := Get("no-such-scenario"); ok || s != nil {
		t.Errorf("Get(unknown) = %v, %v; want nil, false", s, ok)
	}
}

// namedScenario is a Scenario whose name is set by the test.
type namedScenario struct{ coldStart }

func (namedScenario) Name() string { return "" }

func TestRegisterPanics(t *testing.T) {
	cases := []struct {
		name string
		s    Scenario
		want string
	}{
		{"empty name", namedScenario{}, "empty name"},
		{"duplicate name", coldStart{}, `"cold-start" registered twice`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := Names()
			defer func() {
				r := recover()
				msg, _ := r.(string)
				if !strings.Contains(msg, c.want) {
					t.Fatalf("panic = %v, want it to contain %q", r, c.want)
				}
				if after := Names(); !reflect.DeepEqual(before, after) {
					t.Fatalf("registry changed by a refused Register: %v -> %v", before, after)
				}
			}()
			Register(c.s)
		})
	}
}

// g8 are the EPIC G8 probes shared by cold-start and the canon scenarios.
var g8 = []Probe{
	{Metric: "cold_start.version", Args: []string{"version"}, Exit: 0},
	{Metric: "cold_start.help", Args: []string{"--help"}, Exit: 0},
	{Metric: "cold_start.status", Args: []string{"status"}, Exit: 1},
}

// TestProberDefinitions pins the fixed probes of every Prober and proves that
// scenarios which need their own setup (the canon HOME, the HTTP server) are not
// Probers, so `perfbench ab` refuses them instead of measuring the wrong thing.
func TestProberDefinitions(t *testing.T) {
	cases := []struct {
		name   string
		prober bool
		probes []Probe
	}{
		{"cold-start", true, g8},
		{"json-overhead", true, []Probe{
			{Metric: "json_overhead.version", Args: []string{"version", "--json"}, Exit: 0},
			{Metric: "json_overhead.status", Args: []string{"status", "--json"}, Exit: 1},
		}},
		{"cold-start-plugins", false, nil},
		{"cold-start-plugins-v15", false, nil},
		{"http-invoke", false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, ok := Get(c.name)
			if !ok {
				t.Fatal("not registered")
			}
			p, isProber := s.(Prober)
			if isProber != c.prober {
				t.Fatalf("Prober = %v, want %v", isProber, c.prober)
			}
			if !c.prober {
				return
			}
			got := p.Probes()
			if !reflect.DeepEqual(got, c.probes) {
				t.Fatalf("Probes() = %+v, want %+v", got, c.probes)
			}
			checkProbes(t, got)
		})
	}
}

// TestCanonProbes pins the canon scenarios' internal probes: the cold_start
// metric names keep `perfbench check -budget` applicable unchanged.
func TestCanonProbes(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		got := coldStartPlugins{v15: v15}.probes()
		if !reflect.DeepEqual(got, g8) {
			t.Errorf("v15=%v probes() = %+v, want %+v", v15, got, g8)
		}
		checkProbes(t, got)
	}
}

// checkProbes enforces the invariants every probe list must hold.
func checkProbes(t *testing.T, probes []Probe) {
	t.Helper()
	seen := map[string]bool{}
	for _, p := range probes {
		if p.Metric == "" || strings.ContainsAny(p.Metric, " \t") {
			t.Errorf("bad metric name %q", p.Metric)
		}
		if seen[p.Metric] {
			t.Errorf("metric %q appears twice", p.Metric)
		}
		seen[p.Metric] = true
		if len(p.Args) == 0 {
			t.Errorf("%s: no arguments", p.Metric)
		}
		if p.Exit < 0 {
			t.Errorf("%s: negative expected exit %d (that means a signal)", p.Metric, p.Exit)
		}
	}
}
