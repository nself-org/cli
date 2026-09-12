package server

// Purpose: `nself server provision` business logic — validates the request
// and creates the server via Client. Deliberately thin: Hetzner does the
// real validation (bad server_type/location/image), we only add the checks
// that make a confusing provider error unnecessary.
// Inputs: a Client and a ProvisionRequest.
// Outputs: the created Server, or a wrapped error.
// Constraints: always stamps a managed-by=nself-cli label so `list` and a
// future cleanup pass can identify nself-created servers among others in
// the same Hetzner project.
import (
	"context"
	"fmt"
)

// ManagedByLabel marks every server this package creates, so `nself server
// list` (and any future audit) can distinguish nself-created boxes from
// anything else living in the same Hetzner project.
const ManagedByLabel = "managed-by"

// ManagedByValue is ManagedByLabel's value on every server Provision creates.
const ManagedByValue = "nself-cli"

// Provision validates req and creates the server through client.
func Provision(ctx context.Context, client Client, req ProvisionRequest) (*Server, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("provision: --name is required")
	}
	if req.ServerType == "" {
		return nil, fmt.Errorf("provision: --type is required (e.g. cx22)")
	}
	if req.Location == "" {
		return nil, fmt.Errorf("provision: --location is required (e.g. fsn1)")
	}
	if req.Image == "" {
		return nil, fmt.Errorf("provision: --image is required (e.g. ubuntu-24.04)")
	}

	if req.Labels == nil {
		req.Labels = map[string]string{}
	}
	if _, ok := req.Labels[ManagedByLabel]; !ok {
		req.Labels[ManagedByLabel] = ManagedByValue
	}

	srv, err := client.CreateServer(ctx, req)
	if err != nil {
		return nil, err
	}
	return srv, nil
}
