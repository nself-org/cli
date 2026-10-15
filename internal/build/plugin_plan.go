package build

// Purpose: retain installed plugin paths in plans rendered from staging.
// Inputs: the in-memory build and its plan-only plugin source/target.
// Outputs: canonical artifact bytes and effect targets; writes nothing.

import (
	"bytes"
	"strings"
)

func normalizePluginPlan(p *PlannedBuild, opts BuildOptions) {
	if opts.PlanPluginDir == "" || opts.PlanPluginTarget == "" {
		return
	}
	p.PluginTargetDir = opts.PlanPluginTarget
	for path, file := range p.Files {
		file.Data = bytes.ReplaceAll(file.Data, []byte(opts.PlanPluginDir), []byte(opts.PlanPluginTarget))
		p.Files[path] = file
	}
	for i := range p.Effects {
		p.Effects[i].Target = strings.ReplaceAll(p.Effects[i].Target, opts.PlanPluginDir, opts.PlanPluginTarget)
	}
}
