package docker

// network.go — attach and detach a container on a docker network.
//
// Purpose: the shared host proxy (P7-ADOPT-21, EPIC ADOPT D2) connects the
// owner's nginx container to each fronted project's `<project>_network` and
// disconnects it before that project's compose down, which cannot remove a
// network while a foreign container is attached. Compose owns the networks;
// this file never creates, removes or relabels one.
// Inputs: network and container names.
// Outputs: errors that quote the last stderr line. An absent network is a
// bool, "already connected" and "not connected" are success.
// Constraints: names follow "--", so a name that starts with "-" is never read
// as a flag; every docker call stays in this package (the funnel).

import (
	"context"
	"slices"
	"strings"
)

// NetworkExists reports whether a network with exactly this name exists.
func NetworkExists(ctx context.Context, name string) (bool, error) {
	out, err := runCapture(ctx, "network", "inspect", "--format", "{{.Name}}", "--", name)
	if err != nil {
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "no such network") || strings.Contains(low, "network "+strings.ToLower(name)+" not found") {
			return false, nil
		}
		return false, err
	}
	// docker also resolves an id prefix; only the exact name counts here.
	return strings.TrimSpace(string(out)) == name, nil
}

// NetworkConnect connects container to network. A container that is already
// on the network is left alone (no docker connect call).
func NetworkConnect(ctx context.Context, network, container string) error {
	nets, err := ContainerNetworks(ctx, container)
	if err != nil {
		return err
	}
	if slices.Contains(nets, network) {
		return nil
	}
	_, err = runCapture(ctx, "network", "connect", "--", network, container)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "already exists in network") {
		return nil // connected between the check and the call
	}
	return err
}

// NetworkDisconnect disconnects container from network. A container that is
// not on the network, an absent network and an absent container are success.
func NetworkDisconnect(ctx context.Context, network, container string) error {
	nets, err := ContainerNetworks(ctx, container)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such") {
			return nil
		}
		return err
	}
	if !slices.Contains(nets, network) {
		return nil
	}
	_, err = runCapture(ctx, "network", "disconnect", "--", network, container)
	if err != nil {
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "is not connected") || strings.Contains(low, "no such") {
			return nil
		}
	}
	return err
}
