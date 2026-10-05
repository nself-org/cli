package compose

// buildNetworks returns the docker-compose network definitions.
// A single bridge network named {ProjectName}_network is created. Each extra
// network a custom service names in CS_N_NETWORKS is added as an external
// network (it must already exist, e.g. created by another stack of the same
// project); an invalid name is skipped here and reported (E501) by
// buildCustomService, so this function never fails.
func (g *Generator) buildNetworks() map[string]NetworkConfig {
	name := g.cfg.ProjectName + "_network"
	nets := map[string]NetworkConfig{
		name: {
			Driver: "bridge",
			// Explicit Name prevents compose from prefixing the project name,
			// keeping the network identical across regenerations and upgrades.
			Name: name,
		},
	}
	for _, cs := range g.cfg.CustomServices {
		extra, err := customServiceNetworks(g.cfg, cs)
		if err != nil {
			continue
		}
		for _, n := range extra {
			nets[n] = NetworkConfig{Name: n, External: true}
		}
	}
	return nets
}
