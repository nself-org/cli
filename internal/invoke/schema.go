package invoke

// Purpose: derive the exposed flags and the params JSON Schema of a command.
// Inputs: a registry command. Outputs: ExposedFlags and ParamsSchema.
// Constraints: the schema is draft 2020-12 with additionalProperties false at
// every object level; it describes exactly what BuildArgv accepts.

import (
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

// durationPattern is the schema pattern of a duration flag value.
const durationPattern = `^-?([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`

// invokerFlags are root-level switches a request never sets, even when a
// command redeclares one locally.
var invokerFlags = map[string]bool{"json": true, "help": true, "no-monorepo": true, "no-deprecation-warnings": true}

// Flag kinds a request can carry.
const (
	kindBool     = "bool"
	kindInteger  = "integer"
	kindNumber   = "number"
	kindString   = "string"
	kindDuration = "duration"
	kindStrings  = "strings"
)

// flagKind maps a pflag type name to a request kind; "" means not exposable.
func flagKind(pflagType string) string {
	switch pflagType {
	case "bool":
		return kindBool
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "count":
		return kindInteger
	case "float32", "float64":
		return kindNumber
	case "string":
		return kindString
	case "duration":
		return kindDuration
	case "stringSlice", "stringArray":
		return kindStrings
	}
	return ""
}

// ownedFlags are the consent and plan-id flags the invoker alone sets.
func ownedFlags(cmd *cmdregistry.Command) map[string]bool {
	owned := map[string]bool{}
	if cmd.Confirm == nil {
		return owned
	}
	for _, name := range cmd.Confirm.Flags {
		owned[name] = true
	}
	if cmd.Confirm.Plan != nil {
		owned[cmd.Confirm.Plan.Flag] = true
		owned[cmd.Confirm.Plan.IDFlag] = true
	}
	return owned
}

// ExposedFlags lists the flags a request may set, sorted by name. It excludes
// json, help and the other invoker switches, hidden, deprecated and cli_only
// flags, consent and plan-id flags, flags of a type with no request form, and,
// unless transport is TransportHTTPStream, every flag carrying the
// `output: stream` override (HTTP NDJSON is the only place it is accepted).
func ExposedFlags(cmd *cmdregistry.Command, transport string) []cmdregistry.Flag {
	if cmd == nil {
		return nil
	}
	owned := ownedFlags(cmd)
	var out []cmdregistry.Flag
	for _, f := range cmd.Flags {
		stream := f.Output != nil && *f.Output == canon.OutputStream
		switch {
		case invokerFlags[f.Name], owned[f.Name], f.Hidden, f.Deprecated != nil, f.CLIOnly:
		case stream && transport != TransportHTTPStream:
		case flagKind(f.Type) == "":
		default:
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// freeFormArgv reports whether the command takes the free-form argv member: a
// plugin command that declares no args and no flags.
func freeFormArgv(cmd *cmdregistry.Command) bool {
	return cmd.Canon == canon.CanonPlugin && len(cmd.Args) == 0 && len(cmd.Flags) == 0
}

// flagSchema is the schema of one exposed flag, with its registry default.
func flagSchema(f cmdregistry.Flag) map[string]any {
	s := map[string]any{"description": f.Usage}
	switch flagKind(f.Type) {
	case kindBool:
		s["type"] = "boolean"
		if b, err := strconv.ParseBool(f.Default); err == nil {
			s["default"] = b
		}
	case kindInteger:
		s["type"] = "integer"
		if n, err := strconv.ParseInt(f.Default, 10, 64); err == nil {
			s["default"] = n
		}
	case kindNumber:
		s["type"] = "number"
		if n, err := strconv.ParseFloat(f.Default, 64); err == nil {
			s["default"] = n
		}
	case kindDuration:
		s["type"] = "string"
		s["pattern"] = durationPattern
		if f.Default != "" {
			s["default"] = f.Default
		}
	case kindStrings:
		s["type"] = "array"
		s["items"] = map[string]any{"type": "string"}
		s["default"] = []any{}
	default:
		s["type"] = "string"
		s["default"] = f.Default
	}
	return s
}

// argsSchema is the positional-values schema: prefixItems per declared arg.
func argsSchema(cmd *cmdregistry.Command) map[string]any {
	prefix := make([]any, 0, len(cmd.Args))
	required := 0
	variadic := false
	for _, a := range cmd.Args {
		prefix = append(prefix, map[string]any{"type": "string", "description": a.Name})
		if a.Required {
			required++
		}
		variadic = variadic || a.Variadic
	}
	s := map[string]any{"type": "array", "minItems": required}
	if len(prefix) > 0 {
		s["prefixItems"] = prefix
	}
	if variadic {
		s["items"] = map[string]any{"type": "string"}
	} else {
		s["items"] = false
		s["maxItems"] = len(prefix)
	}
	return s
}

// ParamsSchema is the JSON Schema of the machine request for cmd in document
// mode (MCP tool input, HTTP body). A command with a confirm block also
// accepts the confirm member; a free-form plugin command takes argv instead of
// args and flags.
func ParamsSchema(cmd *cmdregistry.Command) map[string]any { return ParamsSchemaIn(nil, cmd) }

// ParamsSchemaIn is ParamsSchema for a command of reg: the free-form argv
// member is offered only on a leaf command (FreeFormArgv). A nil reg takes
// the command as a leaf.
func ParamsSchemaIn(reg *cmdregistry.Registry, cmd *cmdregistry.Command) map[string]any {
	props := map[string]any{}
	if cmd != nil && FreeFormArgv(reg, cmd) {
		props["argv"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"},
			"description": "arguments passed to the plugin for " + strings.TrimPrefix(cmd.Path, "nself ") + " (root flags are refused)"}
	} else if cmd != nil && !freeFormArgv(cmd) {
		flags := map[string]any{}
		for _, f := range ExposedFlags(cmd, TransportMCP) {
			flags[f.Name] = flagSchema(f)
		}
		props["args"] = argsSchema(cmd)
		props["flags"] = map[string]any{"type": "object", "properties": flags, "additionalProperties": false}
	}
	if cmd != nil && cmd.Confirm != nil {
		props["confirm"] = map[string]any{"type": "string", "pattern": confirmPattern.String(),
			"description": "a plan id or a nonce the server issued"}
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
}
