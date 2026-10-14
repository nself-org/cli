package commands

// Purpose: wires the v1 envelope onto the `config bundles` group (P7-SURF-13):
// the group answers with its subcommands, `install` with the same addData as
// `add`. `list` and `info` emit from their own bodies (pre-contract JSON
// stays in v1.4 mode). Data types are bundleListRow, bundleInfoJSON (both
// existing) and addData (install_envelope.go).
// Constraints: registers nothing itself; registry_types_extend.go registers
// the data types. `bundle remove` is the v1.4 spelling of `remove`.
// SPORT: P7-SURF-13, contract:cli.json-envelope

func init() {
	bundleCmd.RunE = dataEnv(bundleCmd.RunE)
	bundleInstallCmd.RunE = outcomeBody(bundleInstallCmd.RunE, addFacts)
}
