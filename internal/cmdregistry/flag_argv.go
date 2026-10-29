package cmdregistry

import (
	"strings"

	"github.com/spf13/pflag"
)

// FlagInsertionIndex finds the position before a positional separator where a
// flag can be inserted, and reports whether name is already forwarded. Required
// flag values are skipped even when they resemble flags.
func FlagInsertionIndex(args []string, name string, local, inherited *pflag.FlagSet) (int, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return i, false
		}
		if !strings.HasPrefix(a, "--") {
			continue
		}
		flagName, _, inlineValue := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if flagName == name {
			return i, true
		}
		flag := local.Lookup(flagName)
		if flag == nil {
			flag = inherited.Lookup(flagName)
		}
		if flag != nil && flag.NoOptDefVal == "" && !inlineValue {
			i++
		}
	}
	return len(args), false
}
