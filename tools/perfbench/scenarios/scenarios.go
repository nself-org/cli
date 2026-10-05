// Package scenarios is the registration API of tools/perfbench
// (contract:cli.perfbench v1). A scenario measures one thing about a built
// nself binary and returns raw samples; perfbench turns the samples into the
// perfbench/v1 JSON document.
//
// Adding a scenario: implement Scenario in a new file of this package and call
// Register from that file's init(). A scenario that probes the binary with
// fixed argument lists should also implement Prober, so `perfbench ab` can
// interleave two binaries over the same probes.
package scenarios

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Config is what perfbench hands to a scenario run.
type Config struct {
	Bin    string   // path of the nself binary under test
	Runs   int      // measured runs per probe
	Warmup int      // discarded runs per probe, before the measured ones
	Env    []string // extra KEY=VALUE entries appended to the probe environment
}

// Sample is one measurement. Every sample of one Metric must carry the same
// Unit; perfbench fails the run otherwise.
type Sample struct {
	Metric string
	Unit   string
	Value  float64
}

// Scenario measures one aspect of the binary.
type Scenario interface {
	Name() string
	Run(ctx context.Context, cfg Config) ([]Sample, error)
}

var (
	mu       sync.Mutex
	registry = map[string]Scenario{}
)

// Register adds s to the registry. It panics on an empty or duplicate name,
// because both are programming errors caught at process start.
func Register(s Scenario) {
	mu.Lock()
	defer mu.Unlock()
	name := s.Name()
	if name == "" {
		panic("perfbench: scenario with empty name")
	}
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("perfbench: scenario %q registered twice", name))
	}
	registry[name] = s
}

// Get returns the scenario registered under name.
func Get(name string) (Scenario, bool) {
	mu.Lock()
	defer mu.Unlock()
	s, ok := registry[name]
	return s, ok
}

// Names lists the registered scenario names, sorted.
func Names() []string {
	mu.Lock()
	defer mu.Unlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
