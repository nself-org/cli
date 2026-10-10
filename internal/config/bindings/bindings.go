// Package bindings declares, once, which command flags supply which
// configuration keys (contract:config.env, Constitution 6.1).
//
// Purpose:     flags.yaml binds a flag to its env key; exempt.yaml records the
//
//	flags that merely look like a key, each with a reason. The
//	command registry fills flag.env from the bindings, `nself
//	config explain` reads them, and the Configuration-Precedence
//	wiki table is rendered from them.
//
// Inputs:      the embedded flags.yaml and exempt.yaml; the known key list
//
//	(config.KnownEnvVars) for validation.
//
// Outputs:     Bindings (Load), the registry map (FlagEnv), lookups, the
//
//	gate (Check) and the wiki table (RenderMarkdown).
//
// Constraints: commands are canonical (v1.5) paths without "nself "; "." is
//
//	the root command. Pure: no I/O beyond the embedded files.
package bindings

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

//go:embed flags.yaml
var flagsYAML []byte

//go:embed exempt.yaml
var exemptYAML []byte

// Root is the Command value of a flag declared on the root command.
const Root = "."

// Binding says that flag Flag of command Command supplies the value of Key.
type Binding struct {
	Command string `yaml:"command"`
	Flag    string `yaml:"flag"`
	Key     string `yaml:"key"`
}

// Exemption records a flag that looks like a key and is not one.
type Exemption struct {
	Command string `yaml:"command"`
	Flag    string `yaml:"flag"`
	Reason  string `yaml:"reason"`
}

// Bindings is the parsed declaration, sorted by command then flag.
type Bindings struct {
	Flags  []Binding
	Exempt []Exemption
}

type flagsFile struct {
	SchemaVersion int       `yaml:"schema_version"`
	Bindings      []Binding `yaml:"bindings"`
}

type exemptFile struct {
	SchemaVersion int         `yaml:"schema_version"`
	Exempt        []Exemption `yaml:"exempt"`
}

var keyShape = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Load parses and validates the embedded declaration against
// config.KnownEnvVars().
func Load() (Bindings, error) {
	return Parse(flagsYAML, exemptYAML, config.KnownEnvVars())
}

// Parse parses flags and exempt documents and validates them against known.
// Every violation is reported in one error.
func Parse(flags, exempt []byte, known []string) (Bindings, error) {
	var ff flagsFile
	var ef exemptFile
	var problems []string
	if err := decodeStrict(flags, &ff); err != nil {
		problems = append(problems, "flags.yaml: "+err.Error())
	}
	if err := decodeStrict(exempt, &ef); err != nil {
		problems = append(problems, "exempt.yaml: "+err.Error())
	}
	if ff.SchemaVersion != 1 {
		problems = append(problems, fmt.Sprintf("flags.yaml: schema_version %d, want 1", ff.SchemaVersion))
	}
	if ef.SchemaVersion != 1 {
		problems = append(problems, fmt.Sprintf("exempt.yaml: schema_version %d, want 1", ef.SchemaVersion))
	}
	isKnown := map[string]bool{}
	for _, k := range known {
		isKnown[k] = true
	}
	b := Bindings{Flags: ff.Bindings, Exempt: ef.Exempt}
	seen := map[string]string{}
	for i, r := range b.Flags {
		at := fmt.Sprintf("flags.yaml bindings[%d] (%s --%s)", i, r.Command, r.Flag)
		switch {
		case r.Command == "" || r.Flag == "" || r.Key == "":
			problems = append(problems, at+": command, flag and key are required")
		case !keyShape.MatchString(r.Key):
			problems = append(problems, fmt.Sprintf("%s: key %q is not UPPER_SNAKE", at, r.Key))
		case !isKnown[r.Key]:
			problems = append(problems, fmt.Sprintf("%s: key %q is not a known configuration key (config.KnownEnvVars)", at, r.Key))
		}
		if prev, dup := seen[ref(r.Command, r.Flag)]; dup {
			problems = append(problems, fmt.Sprintf("%s: already declared (%s)", at, prev))
		}
		seen[ref(r.Command, r.Flag)] = "bound"
	}
	for i, r := range b.Exempt {
		at := fmt.Sprintf("exempt.yaml exempt[%d] (%s --%s)", i, r.Command, r.Flag)
		if r.Command == "" || r.Flag == "" || strings.TrimSpace(r.Reason) == "" {
			problems = append(problems, at+": command, flag and reason are required")
		}
		if prev, dup := seen[ref(r.Command, r.Flag)]; dup {
			problems = append(problems, fmt.Sprintf("%s: already declared (%s); a flag is bound or exempt, not both", at, prev))
		}
		seen[ref(r.Command, r.Flag)] = "exempt"
	}
	if len(problems) > 0 {
		return Bindings{}, fmt.Errorf("config bindings invalid:\n  %s", strings.Join(problems, "\n  "))
	}
	sort.Slice(b.Flags, func(i, j int) bool {
		return less(b.Flags[i].Command, b.Flags[i].Flag, b.Flags[j].Command, b.Flags[j].Flag)
	})
	sort.Slice(b.Exempt, func(i, j int) bool {
		return less(b.Exempt[i].Command, b.Exempt[i].Flag, b.Exempt[j].Command, b.Exempt[j].Flag)
	})
	return b, nil
}

func decodeStrict(doc []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(doc))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func ref(command, flag string) string { return command + "\x00" + flag }

func less(c1, f1, c2, f2 string) bool {
	if c1 != c2 {
		return c1 < c2
	}
	return f1 < f2
}

// FlagEnv returns the registry map: canonical command path to flag name to key.
func (b Bindings) FlagEnv() map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, r := range b.Flags {
		if out[r.Command] == nil {
			out[r.Command] = map[string]string{}
		}
		out[r.Command][r.Flag] = r.Key
	}
	return out
}

// ForKey lists the bindings of one key, in command order.
func (b Bindings) ForKey(key string) []Binding {
	var out []Binding
	for _, r := range b.Flags {
		if r.Key == key {
			out = append(out, r)
		}
	}
	return out
}

// Lookup returns the binding of one flag.
func (b Bindings) Lookup(command, flag string) (Binding, bool) {
	for _, r := range b.Flags {
		if r.Command == command && r.Flag == flag {
			return r, true
		}
	}
	return Binding{}, false
}

// Exemption returns the exemption of one flag.
func (b Bindings) Exemption(command, flag string) (Exemption, bool) {
	for _, r := range b.Exempt {
		if r.Command == command && r.Flag == flag {
			return r, true
		}
	}
	return Exemption{}, false
}
