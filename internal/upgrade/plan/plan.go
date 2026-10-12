// Package plan is the upgrade step model shared by `nself update --plan|--yes`
// and `nself update stack` (cap:cli.update-plan, P7-LIVE-10; P7-PLUG-31 runs
// the same step list, PLUG D10).
//
// Purpose: say, deterministically and without touching anything, which pins of
// a project (the CLI, the CLI-owned images, the installed plugins) differ from
// what the CLI image lock, the plugin registry and the latest CLI release
// offer, and in which order an upgrade applies them.
// Inputs: the project Lock (internal/lock) and the Candidates the caller
// resolved from its sources.
// Outputs: a Plan (the pin diff plus the ordered steps), a human table, and the
// Lock with image pins moved (ApplyImagePins).
// Constraints: pure. No network, no disk, no process. A source that could not
// be reached is the caller's error; a pin the source no longer offers is a Pin
// row with Resolvable=false and a Reason, never an omitted row and never a
// silent default (Constitution section 4.1).
package plan

// Pin kinds.
const (
	KindCLI    = "cli"
	KindImage  = "image"
	KindPlugin = "plugin"
)

// Step names, in execution order.
const (
	StepBackup    = "backup"
	StepImages    = "images"
	StepPlugins   = "plugins"
	StepBuild     = "build"
	StepStart     = "start"
	StepMigrate   = "migrate"
	StepReadiness = "readiness"
	StepHealth    = "health"
	StepCLI       = "cli"
)

// Step kinds say who runs a step. "local" steps change files of the project
// and run in `update --yes`; "stack" steps need a running stack and belong to
// `update stack`; "self" replaces the CLI binary and always runs last.
const (
	StepKindLocal = "local"
	StepKindStack = "stack"
	StepKindSelf  = "self"
)

// Pin is one difference between the project lock and what is on offer.
type Pin struct {
	Kind string `json:"kind"` // cli | image | plugin
	Name string `json:"name"` // "nself" for the CLI, the lock key for images and plugins
	From string `json:"from"` // the current pin or version
	To   string `json:"to"`   // the offered one; empty when unresolvable
	// Resolvable is false when the source no longer offers the pin or answered
	// that it does not exist; Reason then says why.
	Resolvable bool   `json:"resolvable"`
	Reason     string `json:"reason,omitempty"`
	// Note is a caution about a resolvable pin (for example a stateful recreate);
	// it never makes the pin unresolvable.
	Note string `json:"note,omitempty"`
}

// Step is one stage of an upgrade.
type Step struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`    // local | stack | self
	Pins    []Pin  `json:"pins"`    // the pins this step applies (empty for most)
	Command string `json:"command"` // the nself command that performs it
}

// Plan is the pin diff and the ordered steps.
type Plan struct {
	Pins  []Pin  `json:"pins"`
	Steps []Step `json:"steps"`
}

// DefaultSteps returns the ordered step list with each pin attached to the step
// that applies it. Only resolvable pins are attached: an unresolvable one is
// reported in Plan.Pins and never applied.
func DefaultSteps(pins []Pin) []Step {
	var imgs, plugs, cli []Pin
	for _, p := range pins {
		if !p.Resolvable {
			continue
		}
		switch p.Kind {
		case KindImage:
			imgs = append(imgs, p)
		case KindPlugin:
			plugs = append(plugs, p)
		case KindCLI:
			cli = append(cli, p)
		}
	}
	return []Step{
		{StepBackup, StepKindStack, []Pin{}, "nself backup create"},
		{StepImages, StepKindLocal, nonNil(imgs), "rewrite the image pins in nself.lock.yaml"},
		{StepPlugins, StepKindLocal, nonNil(plugs), "nself plugin update <name>"},
		{StepBuild, StepKindLocal, []Pin{}, "nself build"},
		{StepStart, StepKindStack, []Pin{}, "nself start"},
		{StepMigrate, StepKindStack, []Pin{}, "nself db migrate up"},
		{StepReadiness, StepKindStack, []Pin{}, "nself status health"},
		{StepHealth, StepKindStack, []Pin{}, "nself doctor"},
		{StepCLI, StepKindSelf, nonNil(cli), "nself update --cli"},
	}
}

func nonNil(p []Pin) []Pin {
	if p == nil {
		return []Pin{}
	}
	return p
}
