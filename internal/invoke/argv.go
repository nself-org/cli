package invoke

// Purpose: build the child argv from a request without a shell.
// Inputs: a registry command and a Request. Outputs: the argv after the binary.
// Constraints: one request value is exactly one argv element. Layout:
// <path tokens> --json <flags sorted by name> [-- <args>]; a free-form plugin
// command gets <path tokens> --json <argv items>. Names come from the registry
// only; a request can add flags by name, never by spelling. A NUL byte, an
// unknown or unexposed flag, a wrong type or a wrong arg count is E420.

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/cmdregistry"
)

// pathToken is what a registry path word may look like; a word that could be
// read as a flag, or holds a space or control byte, never reaches argv.
var pathToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

// BuildArgv builds the argv for a document-mode request (stream-override flags
// are rejected). Exposure must be checked first.
func BuildArgv(cmd *cmdregistry.Command, r Request) ([]string, error) {
	return BuildArgvFor(cmd, r, TransportHTTP)
}

// BuildArgvFor is BuildArgv for a transport: only TransportHTTPStream accepts
// the stream-override flags.
func BuildArgvFor(cmd *cmdregistry.Command, r Request, transport string) ([]string, error) {
	if cmd == nil {
		return nil, badRequest("no such command")
	}
	path := strings.Fields(barePath(cmd.Path))
	if len(path) == 0 {
		return nil, badRequest("command has no path")
	}
	for _, tok := range path {
		if !pathToken.MatchString(tok) {
			return nil, badRequest("command path %s is not a valid argv word", quoteName(tok))
		}
	}
	if r.Confirm != "" && cmd.Confirm == nil {
		return nil, badRequest("this command takes no confirm")
	}
	argv := append(append([]string{}, path...), "--json")
	if freeFormArgv(cmd) {
		if len(r.Args) > 0 || len(r.Flags) > 0 {
			return nil, badRequest("this command takes argv, not args or flags")
		}
		argv = append(argv, r.Argv...)
		return argv, checkArgv(argv)
	}
	if len(r.Argv) > 0 {
		return nil, badRequest("argv is only for plugin commands that declare no args or flags")
	}
	flagArgv, err := buildFlags(cmd, r.Flags, transport)
	if err != nil {
		return nil, err
	}
	argv = append(argv, flagArgv...)
	if err := checkArgCount(cmd, len(r.Args)); err != nil {
		return nil, err
	}
	if len(r.Args) > 0 {
		argv = append(argv, "--")
		argv = append(argv, r.Args...)
	}
	return argv, checkArgv(argv)
}

// checkArgv rejects NUL bytes and oversized elements or argv.
func checkArgv(argv []string) error {
	total := 0
	for _, a := range argv {
		if strings.IndexByte(a, 0) >= 0 {
			return badRequest("a value contains a NUL byte")
		}
		if len(a) > MaxElementBytes {
			return badRequest("a value is longer than %d bytes", MaxElementBytes)
		}
		total += len(a) + 1
	}
	if total > MaxArgvBytes {
		return badRequest("the request is larger than %d bytes of arguments", MaxArgvBytes)
	}
	return nil
}

func checkArgCount(cmd *cmdregistry.Command, n int) error {
	required, variadic := 0, false
	for _, a := range cmd.Args {
		if a.Required {
			required++
		}
		variadic = variadic || a.Variadic
	}
	switch {
	case n < required:
		return badRequest("too few args: %d given, %d required", n, required)
	case !variadic && n > len(cmd.Args):
		return badRequest("too many args: %d given, at most %d", n, len(cmd.Args))
	}
	return nil
}

// buildFlags renders the request flags in name order.
func buildFlags(cmd *cmdregistry.Command, flags map[string]any, transport string) ([]string, error) {
	exposed := map[string]cmdregistry.Flag{}
	for _, f := range ExposedFlags(cmd, transport) {
		exposed[f.Name] = f
	}
	names := make([]string, 0, len(flags))
	for name := range flags {
		if strings.IndexByte(name, 0) >= 0 {
			return nil, badRequest("a flag name contains a NUL byte")
		}
		if _, ok := exposed[name]; !ok {
			return nil, badRequest("flag %s is unknown or not exposed on this surface", quoteName(name))
		}
		names = append(names, name)
	}
	sort.Strings(names)
	var out []string
	for _, name := range names {
		items, err := flagItems(exposed[name], flags[name])
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

// flagItems renders one flag value as its argv elements.
func flagItems(f cmdregistry.Flag, v any) ([]string, error) {
	bad := func(want string) ([]string, error) {
		return nil, badRequest("flag %s must be %s", quoteName(f.Name), want)
	}
	switch flagKind(f.Type) {
	case kindBool:
		b, ok := v.(bool)
		if !ok {
			return bad("a boolean")
		}
		if b {
			return []string{"--" + f.Name}, nil
		}
		return []string{"--" + f.Name + "=false"}, nil
	case kindInteger:
		s, ok := integerText(v, strings.HasPrefix(f.Type, "uint"))
		if !ok {
			return bad("an integer without a fraction")
		}
		return []string{"--" + f.Name + "=" + s}, nil
	case kindNumber:
		s, ok := numberText(v)
		if !ok {
			return bad("a finite number")
		}
		return []string{"--" + f.Name + "=" + s}, nil
	case kindString:
		s, ok := v.(string)
		if !ok {
			return bad("a string")
		}
		return []string{"--" + f.Name + "=" + s}, nil
	case kindDuration:
		s, ok := v.(string)
		if !ok {
			return bad("a duration string such as 30s")
		}
		if _, err := time.ParseDuration(s); err != nil {
			return bad("a duration string such as 30s")
		}
		return []string{"--" + f.Name + "=" + s}, nil
	case kindStrings:
		return stringItems(f, v)
	}
	return bad("a supported type")
}

// stringItems renders a slice flag as one --name=value per item. pflag splits a
// stringSlice value as CSV, so an item that CSV would split or reject cannot be
// sent faithfully and is refused instead of being silently changed.
func stringItems(f cmdregistry.Flag, v any) ([]string, error) {
	var items []string
	switch x := v.(type) {
	case []string:
		items = x
	case []any:
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, badRequest("flag %s must be an array of strings", quoteName(f.Name))
			}
			items = append(items, s)
		}
	default:
		return nil, badRequest("flag %s must be an array of strings", quoteName(f.Name))
	}
	out := make([]string, 0, len(items))
	for _, s := range items {
		if f.Type == "stringSlice" && strings.ContainsAny(s, ",\"\r\n") {
			return nil, badRequest("flag %s is a comma-separated list: an item cannot hold a comma, quote or newline", quoteName(f.Name))
		}
		out = append(out, "--"+f.Name+"="+s)
	}
	return out, nil
}

// integerText renders v when it is a whole number (and non-negative when
// unsigned). Whole values such as 2.0 and 1e3 are integers (as in JSON Schema);
// 1.5 is not.
func integerText(v any, unsigned bool) (string, bool) {
	var s string
	switch x := v.(type) {
	case json.Number:
		if _, err := strconv.ParseInt(string(x), 10, 64); err == nil {
			s = string(x)
		} else if _, err := strconv.ParseUint(string(x), 10, 64); err == nil {
			s = string(x)
		} else if f, err := strconv.ParseFloat(string(x), 64); err == nil && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
			s = strconv.FormatInt(int64(f), 10)
		} else {
			return "", false
		}
	case float64:
		if x != math.Trunc(x) || math.Abs(x) >= 1<<53 {
			return "", false
		}
		s = strconv.FormatInt(int64(x), 10)
	case int:
		s = strconv.Itoa(x)
	case int64:
		s = strconv.FormatInt(x, 10)
	case uint64:
		s = strconv.FormatUint(x, 10)
	default:
		return "", false
	}
	if unsigned && strings.HasPrefix(s, "-") {
		return "", false
	}
	return s, true
}

func numberText(v any) (string, bool) {
	n, ok := canonNumber(v)
	if !ok {
		return "", false
	}
	return fmt.Sprint(n), true
}
