package reconcile

// Purpose: protect marked output when its ownership record is absent (D-0756).
// Inputs: recorded hashes, current/rendered bytes and explicit force consent.
// Outputs: hand-edit classification, a pruning policy and foreign-file notices.
// Constraints: preserved orphans never enter generated state; v1.4 keeps pruning.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// unrecordedHandEdits compares marked files without a baseline to this render.
// Recorded paths retain the existing callback, including pending env semantics.
func unrecordedHandEdits(state *GeneratedState, before, after ArtifactSet, next func(string) bool) func(string) bool {
	return func(path string) bool {
		recorded := false
		if state != nil {
			_, recorded = state.Files[path]
		}
		if !recorded && IsGenerated(before[path].Data) && !bytes.Equal(before[path].Data, after[path].Data) {
			return true
		}
		return next != nil && next(path)
	}
}

// preserveUnrecordedSites maps disk paths to state keys. Force opts into the
// legacy marker-based sweep; it is independent of the build freshness flag.
func preserveUnrecordedSites(state *GeneratedState, project string, force bool) func(string) bool {
	return func(path string) bool {
		if force {
			return false
		}
		rel, err := filepath.Rel(project, path)
		if err != nil {
			return true
		}
		if state == nil {
			return true
		}
		_, recorded := state.Files[filepath.ToSlash(rel)]
		return !recorded
	}
}

// siteBuildOptions gives read-only Compute the same policy Apply installs.
func siteBuildOptions(req Request) (nbuild.BuildOptions, error) {
	opts := req.Build
	if opts.PreserveNginxSite == nil && compat.V15() {
		state, err := LoadGeneratedState(req.ProjectDir)
		if err != nil {
			return opts, err
		}
		opts.PreserveNginxSite = preserveUnrecordedSites(state, req.ProjectDir, false)
	}
	return opts, nil
}

// warnUnrecordedSites prints only preserved marked orphans: files the render
// writes are adopted normally and must not be described as foreign.
func warnUnrecordedSites(req Request, opts nbuild.BuildOptions, planned *nbuild.PlannedBuild) {
	if req.Stderr == nil || opts.PreserveNginxSite == nil {
		return
	}
	// Pruning only sweeps the project's own sites, never a fronting stack.
	nginxDir, _ := nginxtopo.ServedNginxDir(req.ProjectDir, "")
	sites := filepath.Join(nginxDir, "sites")
	entries, err := os.ReadDir(sites)
	if err != nil {
		return
	} // The build already checked the directory.
	for _, entry := range entries {
		key := "nginx/sites/" + entry.Name()
		path := filepath.Join(sites, entry.Name())
		if _, written := planned.Files[key]; written || entry.IsDir() || !opts.PreserveNginxSite(path) {
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil && IsGenerated(data) {
			_, _ = fmt.Fprintf(req.Stderr, "nself: keeping foreign unrecorded marked file %s; use --force to remove it\n", key)
		}
	}
}
