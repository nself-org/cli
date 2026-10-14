package commands

// Purpose: the v1 envelope for the services canon fragment (`config services`
// and its subcommands; `service ...` is the v1.4 spelling; P7-SURF-13).
// Inputs: a services command body that already ran (writes, lifecycle) or the
// typed facts a read command collects (list, ps, enable/disable state).
// Outputs: one envelope on the real stdout in v1.5 JSON mode only; human
// output stays as it was (the decorator moves it to stderr in JSON mode).
// Constraints: no new behaviour; list keeps its pre-contract array in v1.4 and
// as bare JSON under NSELF_JSON_LEGACY=1; secrets never appear (no env values
// are read into any data type here).
// SPORT: P7-SURF-13, contract:cli.json-envelope

import (
	"strings"

	"github.com/nself-org/cli/internal/service"
	"github.com/spf13/cobra"
)

// serviceListRow is one member of `config services list --json`: a project
// service row (service, enabled, status, port, env_var) or, with --core, a
// catalog row (service, tier, purpose, enable_env, version_env, default_image).
// One type covers both so a single schema describes the array; the fields of
// the other shape are omitted, which keeps each row byte-identical to its
// pre-contract form.
type serviceListRow struct {
	// Service is the service name.
	Service string `json:"service"`
	// Enabled, Status, Port and EnvVar describe a project row.
	Enabled *bool  `json:"enabled,omitempty"`
	Status  string `json:"status,omitempty"`
	Port    string `json:"port,omitempty"`
	EnvVar  string `json:"env_var,omitempty"`
	// Tier, Purpose, EnableEnv, VersionEnv and DefaultImage describe a catalog row (--core).
	Tier         string `json:"tier,omitempty"`
	Purpose      string `json:"purpose,omitempty"`
	EnableEnv    string `json:"enable_env,omitempty"`
	VersionEnv   string `json:"version_env,omitempty"`
	DefaultImage string `json:"default_image,omitempty"`
}

// serviceListData is the data of `config services list --json`.
type serviceListData []serviceListRow

func newServiceListData(status []serviceStatusRow, catalog []catalogRow) serviceListData {
	out := serviceListData{}
	for _, r := range status {
		en := r.Enabled
		out = append(out, serviceListRow{Service: r.Service, Enabled: &en, Status: r.Status, Port: r.Port, EnvVar: r.EnvVar})
	}
	for _, r := range catalog {
		out = append(out, serviceListRow{Service: r.Service, Tier: r.Tier, Purpose: r.Purpose, EnableEnv: r.EnableEnv, VersionEnv: r.VersionEnv, DefaultImage: r.DefaultImage})
	}
	return out
}

// servicePsRow is one container of `config services ps`.
type servicePsRow struct {
	// Name is the container name.
	Name string `json:"name"`
	// Service is the compose service name.
	Service string `json:"service"`
	// Status is the container status text.
	Status string `json:"status"`
	// Health is the health state, "" when the container has no health check.
	Health string `json:"health"`
	// ID is the container id.
	ID string `json:"id"`
}

// servicePsData is the data of `config services ps`.
type servicePsData struct {
	// Services are the stack containers; empty when the stack is not running.
	Services []servicePsRow `json:"services"`
}

// serviceToggleData is the data of `config services enable|disable`: the
// state of the service's *_ENABLED key after the command ran.
type serviceToggleData struct {
	// Service is the canonical service name (aliases resolved).
	Service string `json:"service"`
	// EnvVar is the key that holds the state.
	EnvVar string `json:"env_var"`
	// Enabled is the state after the command: false after a declined disable stays true.
	Enabled bool `json:"enabled"`
}

// servicePsFacts collects the `config services ps` document.
func servicePsFacts(cmd *cobra.Command, _ []string) (any, error) {
	entries, err := service.PS(cmd.Context())
	if err != nil {
		return nil, err
	}
	d := servicePsData{Services: []servicePsRow{}}
	for _, e := range entries {
		d.Services = append(d.Services, servicePsRow{Name: e.Name, Service: e.Service, Status: e.Status, Health: e.Health, ID: e.ID})
	}
	return d, nil
}

// serviceToggleBody runs an enable/disable body, then reports the state the
// env file holds, so a declined dependency prompt reports enabled=true.
func serviceToggleBody(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := run(cmd, args); err != nil {
			return err
		}
		if !jsonEnvelopeOn(cmd) {
			return nil
		}
		canonName, svc, err := canonicalServiceName(args[0])
		if err != nil {
			return err
		}
		envFlag, _ := cmd.Flags().GetString("env")
		envFile, err := resolveEnvFile(envFlag)
		if err != nil {
			return err
		}
		values, err := readEnvValues(envFile)
		if err != nil {
			return err
		}
		on := strings.ToLower(strings.TrimSpace(values[svc.EnvVar])) == "true"
		return emitEnv(cmd, serviceToggleData{Service: canonName, EnvVar: svc.EnvVar, Enabled: on}, false)
	}
}

// readBody answers an envelope run with facts instead of the body: the body
// is the human path. Shared with the plugin reads (plugin_envelope.go).
func readBody(facts func(*cobra.Command, []string) (any, error), run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if !jsonEnvelopeOn(cmd) {
			return run(cmd, args)
		}
		v, err := facts(cmd, args)
		if err != nil {
			return err
		}
		return emitEnv(cmd, v, false)
	}
}

func init() {
	for _, c := range []*cobra.Command{serviceCmd, serviceConfigureCmd, serviceAddCmd, serviceUpgradeCmd,
		serviceStartCmd, serviceStopCmd, serviceRestartCmd, serviceUpdateCmd, serviceScaleCmd} {
		c.RunE = dataEnv(c.RunE)
	}
	serviceEnableCmd.RunE = serviceToggleBody(serviceEnableCmd.RunE)
	serviceDisableCmd.RunE = serviceToggleBody(serviceDisableCmd.RunE)
	servicePsCmd.RunE = readBody(servicePsFacts, servicePsCmd.RunE)
}
