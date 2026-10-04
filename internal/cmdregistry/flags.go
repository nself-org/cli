package cmdregistry

import (
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// localFlags lists the flags declared on c itself (local plus persistent
// declared on c), sorted by name. It reproduces cobra's LocalFlags() rule (a
// flag is inherited when an ancestor's persistent set holds the same *Flag)
// without calling LocalFlags(), which merges parent flags into c.Flags() and
// would mutate the tree. cobra's lazily added `help` flag is never listed: it
// lands on whichever command executes, so listing it would make runtime and
// generated registries differ.
func localFlags(c *cobra.Command) []Flag {
	out := []Flag{}
	seen := map[string]bool{}
	add := func(f *pflag.Flag) {
		if f.Name == "help" || seen[f.Name] || inherited(c, f) {
			return
		}
		seen[f.Name] = true
		out = append(out, toFlag(c, f))
	}
	c.Flags().VisitAll(add)
	c.PersistentFlags().VisitAll(add)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// inherited reports whether f reached c through an ancestor's persistent set.
// The nearest ancestor declaring the name wins, as in cobra.
func inherited(c *cobra.Command, f *pflag.Flag) bool {
	for p := c.Parent(); p != nil; p = p.Parent() {
		if pf := p.PersistentFlags().Lookup(f.Name); pf != nil {
			return pf == f
		}
	}
	return false
}

func toFlag(c *cobra.Command, f *pflag.Flag) Flag {
	required := false
	for _, v := range f.Annotations[cobra.BashCompOneRequiredFlag] {
		if v == "true" {
			required = true
		}
	}
	return Flag{
		Name:       f.Name,
		Shorthand:  strPtr(f.Shorthand),
		Type:       f.Value.Type(),
		Default:    f.DefValue,
		Usage:      f.Usage,
		Hidden:     f.Hidden,
		Deprecated: strPtr(f.Deprecated),
		Required:   required,
		Persistent: c.PersistentFlags().Lookup(f.Name) == f,
	}
}

// strPtr returns nil for the empty string, else a pointer to a copy.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
