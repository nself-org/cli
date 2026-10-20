package compose

// containerName applies the shared project_service container naming convention.
// Inputs: a service name and the generator's configured project name.
// Outputs: the unchanged names joined with one underscore.
// Constraints: preserve existing underscores and custom service names verbatim.
func (g *Generator) containerName(service string) string {
	return g.cfg.ProjectName + "_" + service
}
