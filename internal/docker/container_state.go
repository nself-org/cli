package docker

// container_state.go — read and change the run state of one container.
//
// Purpose: the shared host proxy (P7-ADOPT-21) must attach the owner's nginx
// to fronted networks even when that container is stopped or in a restart loop
// (a static upstream of a fronted project cannot resolve until the network is
// joined, so a cold nginx exits). Docker only allows `network connect` on a
// container that is not restarting, so the caller stops, connects, starts.
// Inputs: a container name.
// Outputs: the Docker state string ("running", "exited", "restarting", ...).
// Constraints: names follow "--"; every docker call stays in this package.

import "context"

// InspectState returns the Docker state of the named container.
func InspectState(ctx context.Context, name string) (string, error) {
	info, err := InspectContainer(ctx, name)
	if err != nil {
		return "", err
	}
	return info.State, nil
}

// StartContainer starts a stopped container (a running one is left alone by
// docker).
func StartContainer(ctx context.Context, name string) error {
	_, err := runCapture(ctx, "start", "--", name)
	return err
}

// StopContainer stops a container; a stopped one is not an error.
func StopContainer(ctx context.Context, name string) error {
	_, err := runCapture(ctx, "stop", "--", name)
	return err
}
