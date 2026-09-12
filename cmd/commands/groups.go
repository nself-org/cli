package commands

import "github.com/spf13/cobra"

// Command groups for `nself --help`.
//
// Purpose: 92 top-level commands rendered as one flat alphabetical wall gave a
// newcomer no way to find the golden path. Cobra groups turn the help output
// into a reading order: the commands that get a stack running come first, and
// everything else sits under the concern it belongs to.
//
// Inputs: none — this is registration data.
//
// Outputs: cobra.Group registrations on RootCmd, plus commandGroupAssignments,
// the single table mapping command name to group.
//
// Constraints: every visible top-level command must have a GroupID. Ungrouped
// commands land in cobra's "Additional Commands" bucket, which is the flat wall
// again for that subset, so TestEveryCommandHasAGroup fails on any command
// missing from the table below.
const (
	groupCore     = "core"
	groupConfig   = "config"
	groupData     = "data"
	groupDeploy   = "deploy"
	groupObserve  = "observe"
	groupExtend   = "extend"
	groupAccount  = "account"
	groupAdvanced = "advanced"
)

// commandGroups defines the groups and the order they appear in help output.
var commandGroups = []*cobra.Group{
	{ID: groupCore, Title: "Core — get a stack running:"},
	{ID: groupConfig, Title: "Configuration & Environment:"},
	{ID: groupData, Title: "Data:"},
	{ID: groupDeploy, Title: "Deploy & Remote:"},
	{ID: groupObserve, Title: "Observability:"},
	{ID: groupExtend, Title: "Plugins & Extensions:"},
	{ID: groupAccount, Title: "Account & Meta:"},
	{ID: groupAdvanced, Title: "Advanced & Enterprise:"},
}

// commandGroupAssignments maps every visible top-level command to its group.
// Adding a command without adding it here fails TestEveryCommandHasAGroup.
var commandGroupAssignments = map[string]string{
	// Core — the golden path plus the commands you reach for while running it.
	"init":    groupCore,
	"build":   groupCore,
	"start":   groupCore,
	"stop":    groupCore,
	"restart": groupCore,
	"status":  groupCore,
	"logs":    groupCore,
	"urls":    groupCore,
	"exec":    groupCore,
	"doctor":  groupCore,
	"clean":   groupCore,
	"reset":   groupCore,
	"dev":     groupCore,

	// Configuration & Environment.
	"config":   groupConfig,
	"env":      groupConfig,
	"secrets":  groupConfig,
	"service":  groupConfig,
	"trust":    groupConfig,
	"generate": groupConfig,
	"template": groupConfig,

	// Data.
	"db":        groupData,
	"backup":    groupData,
	"migrate":   groupData,
	"functions": groupData,

	// Deploy & Remote.
	"deploy":  groupDeploy,
	"promote": groupDeploy,
	"ci":      groupDeploy,
	"ops":     groupDeploy,

	// Observability.
	"health":    groupObserve,
	"self-heal": groupObserve,

	// Plugins & Extensions.
	"plugin":  groupExtend,
	"install": groupExtend,
	"remove":  groupExtend,
	"bundle":  groupExtend,
	"license": groupExtend,
	"mcp":     groupExtend,

	// Account & Meta.
	"account":     groupAccount,
	"login":       groupAccount,
	"logout":      groupAccount,
	"oauth":       groupAccount,
	"admin":       groupAccount,
	"update":      groupAccount,
	"version":     groupAccount,
	"completion":  groupAccount,
	"man":         groupAccount,
	"help-topics": groupAccount,
	"telemetry":   groupAccount,

	// Advanced & Enterprise.
	"access":      groupAdvanced,
	"security":    groupAdvanced,
	"server":      groupAdvanced,
	"verify-sbom": groupAdvanced,
	"runner":      groupAdvanced,
}

// ApplyCommandGroups registers the groups and assigns each command to one.
// Called from Execute() rather than init(): every command's own init() must
// have run and registered it on RootCmd before assignments can be applied, and
// init() ordering within a package is by filename, which is not something to
// depend on.
//
// Exported so the doc generators in tools/ produce the same grouping the binary
// shows; they walk RootCmd directly and never call Execute.
// Idempotent — safe to call more than once.
func ApplyCommandGroups() {
	if len(RootCmd.Groups()) == 0 {
		RootCmd.AddGroup(commandGroups...)
	}
	for _, c := range RootCmd.Commands() {
		if c.GroupID != "" {
			continue
		}
		if id, ok := commandGroupAssignments[c.Name()]; ok {
			c.GroupID = id
		}
	}
}
