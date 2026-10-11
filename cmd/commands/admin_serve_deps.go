package commands

// Purpose: connect the admin HTTP API to the shared machine registry.
// Inputs: composition-root project resolver and embedded schemas.
// Outputs: ServeDeps and the exported generator registry provider.
// Constraints: build the machine registry only at server/generator startup.

import (
	"strings"

	"github.com/nself-org/cli/internal/builtin/admin"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/schemas"
)

// MachineRegistry exposes the existing machine registry for hermetic generators.
func MachineRegistry() (*cmdregistry.Registry, error) { return machineRegistry() }

// Assign the provider after variable initialization: building the registry
// mounts builtinFamilies, whose builder creates these dependencies.
var adminServeRegistry func() (*cmdregistry.Registry, error)

func init() { adminServeRegistry = machineRegistry }

func adminServeDeps() admin.ServeDeps {
	return admin.ServeDeps{
		Registry: adminServeRegistry, ProjectDir: resolveProjectDir,
		DataSchema: func(path string) ([]byte, bool) {
			return schemas.Lookup(strings.TrimPrefix(path, "schemas/"))
		},
	}
}
