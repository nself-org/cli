package imageswitch

// leftover.go — the data volumes a finished switch leaves behind.
//
// Purpose: after a switch the compose file names only the new volume, so
// `compose down --volumes` (reset, stop --volumes, uninstall --purge) cannot
// remove the kept old volume or the volumes of earlier switches. This finds
// them, scoped to this project, so those commands can wipe all project data.
// Inputs: the project directory (its switch state names the compose project).
// Outputs: volume names; none when the project never switched.
// Constraints: never lists volumes of another compose project; a volume whose
// compose project label names a different project is skipped; list errors are
// returned, never swallowed.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/nself-org/cli/internal/docker"
)

func init() {
	docker.VolumesRemovedHook = func(ctx context.Context, workdir string) error {
		_, err := RemoveLeftovers(ctx, workdir)
		return err
	}
}

const labelComposeProject = "com.docker.compose.project"

// LeftoverVolumes returns the project's data volumes that a switch kept: the
// old volume recorded in the state, and every volume of the same compose
// project carrying the switch label (earlier switches' new volumes).
func LeftoverVolumes(ctx context.Context, projectDir string) ([]string, error) {
	st, err := LoadState(projectDir)
	if err != nil || st == nil || st.ComposeProject == "" {
		return nil, err
	}
	names, err := docker.VolumeListByLabels(ctx, labelRun, labelComposeProject+"="+st.ComposeProject)
	if err != nil {
		return nil, err
	}
	if old := st.OldVolume; old != "" && !slices.Contains(names, old) {
		labels, exists, err := docker.VolumeLabels(ctx, old)
		if err != nil {
			return nil, err
		}
		if p := labels[labelComposeProject]; exists && (p == "" || p == st.ComposeProject) {
			names = append(names, old)
		}
	}
	return names, nil
}

// RemoveLeftovers removes LeftoverVolumes and returns the names removed. A list
// or removal failure is returned (all removals are still attempted), so the
// caller never reports the data as gone when it is not.
func RemoveLeftovers(ctx context.Context, projectDir string) ([]string, error) {
	names, err := LeftoverVolumes(ctx, projectDir)
	if err != nil {
		return nil, fmt.Errorf("listing image-switch volumes: %w", err)
	}
	var removed []string
	var errs []error
	for _, v := range names {
		if err := docker.VolumeRemove(ctx, v); err != nil {
			errs = append(errs, fmt.Errorf("removing image-switch volume %s: %w", v, err))
			continue
		}
		removed = append(removed, v)
	}
	return removed, errors.Join(errs...)
}
