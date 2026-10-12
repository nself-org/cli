package build

// lockfile_cache.go — the cache engine of an existing project in the build
// pipeline (P7-LIVE-19, EPIC D16, review F14).
//
// Purpose: the lock bootstrap pins only literal image references, so a redis
// image written as redis:${REDIS_VERSION:-7-alpine}, a built redis service or a
// bare redis_data volume left no pin and a v1.5 build picked Valkey, a new empty
// volume. When the lock has no redis pin, the existing docker-compose.yml is the
// evidence of a running cache and decides the engine.
// Inputs: the project's docker-compose.yml (through the sink) and the installed pins.
// Outputs: compose.SetExistingCacheEngine for the render.
// Constraints: an explicit REDIS_ENGINE and a redis pin both outrank this hint
// (compose.cacheEngine); no compose file or no cache in it means no hint.

import (
	"path/filepath"

	"github.com/nself-org/cli/internal/compose"
)

// installLockPins installs the image pins for this render, plus the cache engine
// the existing compose shows when the lock has no redis pin, and returns the
// function that restores the previous state.
func (st *buildState) installLockPins() (func(), error) {
	restorePins, err := st.installImagePins()
	if err != nil {
		return nil, err
	}
	hint := ""
	if _, pinned := compose.ProjectPins()["redis"]; !pinned {
		if data, rerr := st.sink.ReadFile(filepath.Join(st.workdir, "docker-compose.yml")); rerr == nil {
			hint = compose.ExistingCacheEngine(data)
		}
	}
	restoreHint := compose.SetExistingCacheEngine(hint)
	return func() { restoreHint(); restorePins() }, nil
}
