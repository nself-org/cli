package commands

// config_explain_args.go — the hand-parsed command line of `nself config explain`.
//
// Purpose: flag parsing is off for `config explain` because the target flag
//          (`--domain`) is data, not an option of explain. This file reads the
//          raw arguments: the options --reveal, --json and -h/--help, the root
//          persistent flags, the command words and the one target flag.
// Inputs:  the raw argument list cobra passes with DisableFlagParsing.
// Outputs: explainInput, or E401 usage for arguments that cannot be read.
// Constraints: boolean values are strconv.ParseBool values; a root persistent
//          flag is an option unless it is the only flag after command words;
//          a KEY takes no target flag. Pure except for reading RootCmd's flag
//          set and setting flags in prepareConfigExplain.

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// explainInput is the hand-parsed command line.
type explainInput struct {
	words  []string
	flag   string
	reveal bool
	json   bool
	help   bool
	// globals holds the root persistent flags given (name -> "true"/"false"),
	// so prepareConfigExplain can apply them as cobra would have.
	globals map[string]string
}

func explainUsage(format string, a ...any) error {
	return errs.Newf("E401", format, a...).WithFix("Usage: nself config explain KEY, or nself config explain <command> --<flag>.")
}

// explainKeyShape matches a configuration KEY (BASE_DOMAIN). Command words are
// lower case, so the shape alone tells the KEY form from the command form.
var explainKeyShape = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// explainBool reads the value of a boolean option: "--name" is true and
// "--name=<v>" is strconv.ParseBool(v).
func explainBool(name, value string, hasValue bool) (bool, error) {
	if !hasValue {
		return true, nil
	}
	v, err := strconv.ParseBool(value)
	if err != nil {
		return false, explainUsage("--%s takes a boolean, got %q", name, value)
	}
	return v, nil
}

// isRootPersistentFlag reports whether name is a persistent flag of the root
// command (--no-monorepo, --no-deprecation-warnings; --json is read apart).
func isRootPersistentFlag(name string) bool {
	return name != "json" && RootCmd.PersistentFlags().Lookup(name) != nil
}

// parseExplainArgs reads the raw arguments. --reveal, --json and -h/--help are
// options of explain; their values are parsed with strconv.ParseBool. A root
// persistent flag is an option too, unless it is the only flag after command
// words, where it is the flag being asked about. Any other --name is the
// target flag. A KEY takes no target flag.
func parseExplainArgs(args []string) (explainInput, error) {
	var in explainInput
	var globals []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			in.help = true
		case strings.HasPrefix(a, "--"):
			name, value, hasValue := strings.Cut(strings.TrimPrefix(a, "--"), "=")
			switch {
			case name == "":
				continue // a bare "--" ends nothing here
			case name == "reveal" || name == "json":
				v, err := explainBool(name, value, hasValue)
				if err != nil {
					return in, err
				}
				if name == "reveal" {
					in.reveal = v
				} else {
					in.json = v
				}
			case isRootPersistentFlag(name):
				v, err := explainBool(name, value, hasValue)
				if err != nil {
					return in, err
				}
				if in.globals == nil {
					in.globals = map[string]string{}
				}
				in.globals[name] = strconv.FormatBool(v)
				globals = append(globals, name)
			default:
				if in.flag != "" {
					return in, explainUsage("config explain takes one flag, got --%s and --%s", in.flag, name)
				}
				in.flag = name
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return in, explainUsage("unknown option %q: only --reveal, --json and one target --flag are accepted", a)
		default:
			in.words = append(in.words, a)
		}
	}
	if len(in.words) > 1 && in.words[0] == "nself" {
		in.words = in.words[1:]
	}
	isKey := len(in.words) == 1 && explainKeyShape.MatchString(in.words[0])
	if in.flag == "" && !isKey && len(in.words) > 0 && len(globals) > 0 {
		// `config explain build --no-monorepo` asks about the root flag itself.
		if len(globals) > 1 {
			return in, explainUsage("config explain takes one flag, got --%s and --%s", globals[0], globals[1])
		}
		in.flag = globals[0]
		delete(in.globals, in.flag)
	}
	if isKey && in.flag != "" {
		return in, explainUsage("%s is a KEY and takes no flag; drop --%s, or ask nself config explain <command> --%s", in.words[0], in.flag, in.flag)
	}
	return in, nil
}

// prepareConfigExplain turns on the persistent json flag before the invocation
// decorator runs the body, because flag parsing is disabled.
func prepareConfigExplain(cmd *cobra.Command, args []string) error {
	in, err := parseExplainArgs(args)
	if err != nil {
		return err
	}
	_ = cmd.InheritedFlags() // merge the persistent flags into cmd.Flags()
	if in.json {
		if err := cmd.Flags().Set("json", "true"); err != nil {
			return err
		}
	}
	for name, v := range in.globals {
		if cmd.Flags().Lookup(name) == nil {
			continue // a command built outside the root tree (tests)
		}
		if err := cmd.Flags().Set(name, v); err != nil {
			return err
		}
	}
	return nil
}
