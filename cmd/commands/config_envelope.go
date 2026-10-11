package commands

// Purpose: the v1 envelope wiring of the config canon fragment (config, config
// env/secrets/oauth/telemetry/trust/features/validate/export/import;
// P7-SURF-12). It wraps every RunE of the fragment with dataEnv (the shared
// data-fragment emitter), gives the pre-contract JSON rows their v1.5 form and
// holds the small helpers the bodies share.
// Inputs: the cobra commands of the fragment, wrapped from init(); the files
// that own `config features *` (feature.go) and `config trust ssl *` (ssl*.go)
// stay untouched, their package-level commands are wrapped here.
// Outputs: one envelope on stdout in v1.5 mode with --json (or --format json
// for export); v1.4 output and every human output are unchanged.
// Constraints: a secret value never reaches an envelope except through the two
// CLI-only rows (config secrets get, decrypt-on-deploy). URL credentials in
// echoed values are masked (config.MaskURLCredentials).
// SPORT: P7-SURF-12, contract:cli.json-envelope

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// configDataEnvCmds are the commands whose RunE is wrapped with dataEnv: the
// ones that hand their data over with setData or take the shared result/hub.
// The self-emitting rows (config export, env target list/probe, features
// list/status) are not here; their bodies call emitEnv themselves.
func configDataEnvCmds() []*cobra.Command {
	return []*cobra.Command{
		configCmd, configImportCmd, configValidateCmd,
		envCmd, envUseCmd, envListCmd, envShowCmd, envDiffCmd, envCopyCmd, envExplainCmd,
		envTargetCmd, envTargetAddCmd, envTargetRemoveCmd, envTargetMigrateCmd,
		featureCmd, featureEnableCmd, featureDisableCmd,
		oauthCmd, oauthRefreshCmd,
		secretsCmd, secretsAuditCmd, secretsDecryptOnDeployCmd, secretsGetCmd, secretsInitCmd,
		secretsLintCmd, secretsListCmd, secretsListSchedulesCmd, secretsRekeyCmd, secretsRetireCmd,
		secretsRotateCmd, secretsRotationLogCmd, secretsScheduleCmd, secretsSetCmd, secretsVerifyCmd,
		telemetryCmd, telemetryOffCmd, telemetryOnCmd, telemetryStatusCmd,
		trustCmd, trustStatusCmd, dnsSetupCmd,
		sslCmd, sslStatusCmd, sslRenewCmd, sslSetupCmd, sslAddCmd,
	}
}

func init() {
	for _, c := range configDataEnvCmds() {
		c.RunE = dataEnv(c.RunE)
	}
	wrapFeatureLegacyJSON()
}

// wrapFeatureLegacyJSON gives `config features list|status` their v1.5 form
// from outside feature.go: when the invocation answers with an envelope the
// wrapper computes the same data the body would print and emits it through the
// legacy-compatible path (bare with NSELF_JSON_LEGACY=1); otherwise the body
// runs unchanged. The body's own --json branch is unreachable in that mode
// (the decorator isolates stdout, so its bare print would land on stderr).
func wrapFeatureLegacyJSON() {
	list, status := featureListCmd.RunE, featureStatusCmd.RunE
	featureListCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !jsonEnvelopeOn(cmd) {
			return list(cmd, args)
		}
		reg, state, _, err := loadRegAndState(cmd)
		if err != nil {
			return fmt.Errorf("feature list: %w", err)
		}
		return emitEnv(cmd, legacyList(reg.Resolve(state)), true)
	}
	featureStatusCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !jsonEnvelopeOn(cmd) {
			return status(cmd, args)
		}
		reg, state, _, err := loadRegAndState(cmd)
		if err != nil {
			return fmt.Errorf("feature status: %w", err)
		}
		st, ok := reg.ResolveOne(state, args[0])
		if !ok {
			return fmt.Errorf("feature status: unknown flag %q (run `nself feature list`)", args[0])
		}
		return emitEnv(cmd, st, true)
	}
}

// legacyList returns rows for the envelope. The bare pre-contract output of an
// empty list is `null` (a nil slice); the envelope data is an array, so a nil
// list becomes [] unless the bare form is still requested (v1.5 mode with
// NSELF_JSON_LEGACY=1 keeps `null`, as v1.4 prints it).
func legacyList[T any](rows []T) []T {
	// compat.V15(P7-SURF-12): an empty list is the JSON array [] (v1.4 and NSELF_JSON_LEGACY=1 print null) -> v1 envelope with an array
	if rows == nil && compat.V15() && !legacyJSONRequested() {
		return []T{}
	}
	return rows
}

// machineInvoked reports that this process runs as the child of the MCP or HTTP
// invoker (NSELF_INVOKED_BY is set by internal/invoke).
func machineInvoked() bool { return os.Getenv("NSELF_INVOKED_BY") != "" }

// refuseRevealForMachine is the defence in depth behind the cli_only marking of
// every --reveal flag: a machine-invoked child never prints a secret value,
// whatever argv shape asked for it.
func refuseRevealForMachine() error {
	return errs.New("E421", "--reveal is not available over MCP or HTTP: secret values never leave the process")
}
