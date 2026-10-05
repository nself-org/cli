package manifestv2

// Commands is the CLI surface block (v1 `cli` is a different key and type).
// The optional root summary, side_effect, output and json decode and round-trip;
// absent they mean side_effect destructive and json none (CANON mount contract).
type Commands struct {
	Command     string       `json:"command"`
	Binary      string       `json:"binary"`
	Summary     string       `json:"summary,omitempty"`
	SideEffect  string       `json:"side_effect,omitempty"`
	Output      string       `json:"output,omitempty"`
	JSON        string       `json:"json,omitempty"`
	Confirm     *Confirm     `json:"confirm,omitempty"`
	Surface     string       `json:"surface,omitempty"`
	Subcommands []Subcommand `json:"subcommands,omitempty"`
}

// Subcommand is one nested command path below the root.
type Subcommand struct {
	// Name is one or more space-separated segments.
	Name       string   `json:"name"`
	Summary    string   `json:"summary,omitempty"`
	SideEffect string   `json:"side_effect,omitempty"`
	Output     string   `json:"output,omitempty"`
	JSON       string   `json:"json,omitempty"`
	Args       []Arg    `json:"args,omitempty"`
	Flags      []Flag   `json:"flags,omitempty"`
	Confirm    *Confirm `json:"confirm,omitempty"`
	Surface    string   `json:"surface,omitempty"`
}

// Arg is a positional argument (registry Arg shape plus secret).
type Arg struct {
	Name     string `json:"name"`
	Required bool   `json:"required,omitempty"`
	Variadic bool   `json:"variadic,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
}

// Flag is a flag (registry Flag shape plus secret and cli_only).
type Flag struct {
	Name       string `json:"name"`
	Shorthand  string `json:"shorthand,omitempty"`
	Type       string `json:"type"`
	Default    string `json:"default,omitempty"`
	Usage      string `json:"usage,omitempty"`
	Hidden     bool   `json:"hidden,omitempty"`
	Required   bool   `json:"required,omitempty"`
	Persistent bool   `json:"persistent,omitempty"`
	Env        string `json:"env,omitempty"`
	SideEffect string `json:"side_effect,omitempty"`
	JSON       string `json:"json,omitempty"`
	Output     string `json:"output,omitempty"`
	Secret     bool   `json:"secret,omitempty"`
	CLIOnly    bool   `json:"cli_only,omitempty"`
}

// Confirm names the bool flags that confirm a destructive action. An empty
// Flags list is valid: a nonce-only round trip (Epic D16).
type Confirm struct {
	Flags []string `json:"flags"`
	Plan  *Plan    `json:"plan,omitempty"`
}

// Plan names the flags of a plan/apply pair of one command.
type Plan struct {
	Flag   string `json:"flag"`
	IDFlag string `json:"id_flag"`
}
