package scenarios

import "context"

// jsonOverhead measures the machine-readable invocations, so a change that
// wraps every command's --json path (the P7-REG-05 decorator) can be compared
// base against head with `perfbench ab -scenario json-overhead`. The status
// probe runs in an empty directory and is expected to exit non-zero.
type jsonOverhead struct{}

func init() { Register(jsonOverhead{}) }

func (jsonOverhead) Name() string { return "json-overhead" }

func (jsonOverhead) Probes() []Probe {
	return []Probe{
		{Metric: "json_overhead.version", Args: []string{"version", "--json"}},
		{Metric: "json_overhead.status", Args: []string{"status", "--json"}},
	}
}

func (j jsonOverhead) Run(ctx context.Context, cfg Config) ([]Sample, error) {
	return RunProbes(ctx, cfg, j.Probes())
}
