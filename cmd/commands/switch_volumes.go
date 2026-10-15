package commands

// switch_volumes.go — remove the volumes a postgres image switch keeps (D-0802).
//
// Purpose: after `nself db image switch` the generated compose file names only
// the NEW postgres volume, so `compose down --volumes` leaves the kept OLD
// volume (a full copy of the data) behind, and after a rollback it leaves the
// new one. A data wipe (reset, uninstall --purge, stop --volumes) must take
// both with it.
// Inputs: the project directory (its .nself/image-switch/state.json) and the
// docker volumes labeled org.nself.image-switch for the same compose project.
// Outputs: those volumes removed; an error naming every one that could not be.
// Constraints: only runs after the wipe was confirmed and compose down
// succeeded; a project that never switched images has no state and is left
// alone; an absent volume is success.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/postgres/imageswitch"
)

// switchVolumeLabel is put on the new volume (and the one-shots) by the switch.
const switchVolumeLabel = "org.nself.image-switch"

// Seams so tests need no daemon beyond the fake docker on PATH.
var (
	volumesByLabel = docker.VolumesByLabel
	volumeRemove   = docker.VolumeRemove
)

// removeSwitchVolumes removes the old and new volumes recorded by an image
// switch in projectDir, plus any volume labeled org.nself.image-switch under
// the same compose project.
func removeSwitchVolumes(ctx context.Context, projectDir string) error {
	st, err := imageswitch.LoadState(projectDir)
	if err != nil {
		return fmt.Errorf("reading the image switch state: %w", err)
	}
	if st == nil {
		return nil
	}
	var names []string
	for _, n := range []string{st.OldVolume, st.NewVolume} {
		if n != "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	if st.ComposeProject != "" {
		labeled, lerr := volumesByLabel(ctx, switchVolumeLabel, "com.docker.compose.project="+st.ComposeProject)
		if lerr != nil {
			return fmt.Errorf("listing image switch volumes: %w", lerr)
		}
		for _, n := range labeled {
			if !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
	}
	var errs []error
	for _, n := range names {
		if err := volumeRemove(ctx, n); err != nil {
			errs = append(errs, fmt.Errorf("removing volume %s: %w", n, err))
		}
	}
	return errors.Join(errs...)
}
