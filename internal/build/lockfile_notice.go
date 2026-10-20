package build

// lockfile_notice.go — the notice for a project that loses its hasura pin source (D-0719).
//
// Purpose: a v1.5 build with no nself.lock.yaml and no docker-compose.yml takes
// the CLI default hasura; an existing project is told so and where its backup step is.
// Inputs: the build state (config, sink) and the docker volume check.
// Outputs: one warning per build through warnOnce.
// Constraints: a new project (no postgres volume) and a v1.4 build stay silent.

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/lock"
)

// projectPostgresVolumeExists reports whether the project's postgres data
// volume exists, through the docker funnel. It is a variable so tests can fake
// it. A docker error counts as "no": the notice it gates is advisory.
var projectPostgresVolumeExists = func(name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := docker.VolumeExists(ctx, name)
	return err == nil && ok
}

// noticeUnpinnedHasura warns, once per build, that a project with data but no
// pin source (no lock, no docker-compose.yml: nself clean keeps data and
// removes both) takes the CLI default hasura, which in v1.5 is a newer
// graphql-engine than the v1.4 default (D-0719). It names the move and the
// backup step. A project without a postgres volume is new and gets no notice.
func (st *buildState) noticeUnpinnedHasura() {
	if st.cfg == nil || compose.HasuraEntry() == "hasura-v14" {
		return
	}
	vol := strings.TrimSpace(st.cfg.Postgres.DataVolume)
	if vol == "" {
		vol = st.cfg.ProjectName + "_postgres_data"
	}
	if !projectPostgresVolumeExists(vol) {
		return
	}
	from, _ := compose.LockedRef("hasura-v14")
	to, _ := compose.LockedRef("hasura")
	st.warnOnce("no "+lock.FileName+" and no docker-compose.yml, but this project's database exists: hasura takes the v1.5 default graphql-engine "+to.Version+" (v1.4 ran "+from.Version+"); run `nself backup create` first (going back is restore-then-pin-back, see the Image-Lock wiki page, Roll back)", "volume", vol)
}
