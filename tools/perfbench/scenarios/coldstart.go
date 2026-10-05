package scenarios

import "context"

// coldStart is the default scenario (EPIC G8): wall time of a fresh nself
// process for three probes. `version` is the subcommand, so the row matches
// the declared "nself version / nself help" SLO; `status` runs in an empty
// directory and is expected to exit non-zero (the exit code is recorded and
// must not change between runs).
type coldStart struct{}

func init() { Register(coldStart{}) }

func (coldStart) Name() string { return "cold-start" }

func (coldStart) Probes() []Probe {
	return []Probe{
		{Metric: "cold_start.version", Args: []string{"version"}},
		{Metric: "cold_start.help", Args: []string{"--help"}},
		{Metric: "cold_start.status", Args: []string{"status"}},
	}
}

func (c coldStart) Run(ctx context.Context, cfg Config) ([]Sample, error) {
	return RunProbes(ctx, cfg, c.Probes())
}
