package mount

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

var (
	segmentRe = regexp.MustCompile(manifestv2.NamePattern)
	binaryRe  = regexp.MustCompile(manifestv2.BinaryPattern)
)

// Discover lists the mountable command surface of every installed, enabled
// plugin in pluginDir. verbs are the ADR 0016 core command verbs, passed in
// by the caller from the generated canon table (never canon.Load): a plugin
// may not claim one. Specs are sorted by slug; a plugin that cannot mount
// contributes a Problem instead and is left out.
func Discover(pluginDir string, verbs []string) ([]Spec, []Problem) {
	dir := pluginDir
	if resolved, err := filepath.EvalSymlinks(pluginDir); err == nil {
		dir = resolved
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		// No plugins directory at all is a clean install, not a problem.
		return nil, nil
	}
	var specs []Spec
	var problems []Problem
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "bin" {
			continue
		}
		slug := e.Name()
		if plugin.IsDisabled(slug, dir) {
			continue
		}
		spec, p := discoverOne(dir, slug, verbs)
		if p != nil {
			problems = append(problems, *p)
			continue
		}
		if spec != nil {
			specs = append(specs, *spec)
		}
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Slug < specs[j].Slug })
	specs = dropDuplicateCommands(specs, &problems)
	return specs, problems
}

// discoverOne reads one plugin's manifest. A nil Spec and nil Problem means
// the plugin declares no commands block, so there is nothing to mount.
func discoverOne(dir, slug string, verbs []string) (*Spec, *Problem) {
	path := filepath.Join(dir, slug, "plugin.json")
	if _, err := os.Stat(path); err != nil {
		// A directory without plugin.json is not a CLI plugin.
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &Problem{Code: codeOf(err), Slug: slug, Message: fmt.Errorf("reading plugin manifest: %w", err).Error()}
	}
	m, err := manifestv2.ParseQuiet(data)
	if err != nil {
		return nil, &Problem{Code: codeOf(err), Slug: slug, Message: fmt.Errorf("%s: %w", path, err).Error()}
	}
	c := m.Commands
	if c == nil {
		return nil, nil
	}
	if p := validateSpec(slug, c, verbs); p != nil {
		return nil, p
	}
	binPath, p := resolveBinary(dir, slug, c.Binary)
	if p != nil {
		return nil, p
	}
	return &Spec{
		Slug: slug, Command: c.Command, Binary: c.Binary, BinaryPath: binPath,
		Summary: c.Summary, SideEffect: c.SideEffect, Output: c.Output, JSON: c.JSON,
		Confirm: rawConfirm(c.Confirm), Surface: c.Surface,
		Subcommands: subs(c.Subcommands),
	}, nil
}

// validateSpec applies the mount rules the v1 normalizer cannot (Normalize
// runs no v2 Validate): name patterns, the core-verb exclusion and duplicate
// subcommand paths.
func validateSpec(slug string, c *manifestv2.Commands, verbs []string) *Problem {
	if !segmentRe.MatchString(c.Command) {
		return invalid(slug, "commands.command", "must match "+manifestv2.NamePattern)
	}
	for _, v := range verbs {
		if c.Command == v {
			return collision(slug, fmt.Sprintf("command %q collides with the core command verb %q", c.Command, v))
		}
	}
	if !binaryRe.MatchString(c.Binary) {
		return invalid(slug, "commands.binary", "must match "+manifestv2.BinaryPattern)
	}
	seen := map[string]bool{}
	for _, s := range c.Subcommands {
		for _, seg := range strings.Fields(s.Name) {
			if !segmentRe.MatchString(seg) {
				return invalid(slug, "commands.subcommands", fmt.Sprintf("segment %q must match %s", seg, manifestv2.NamePattern))
			}
		}
		if seen[s.Name] {
			return invalid(slug, "commands.subcommands", fmt.Sprintf("%q is declared twice", s.Name))
		}
		seen[s.Name] = true
	}
	return nil
}

// resolveBinary checks the declared binary: it must resolve (after symlinks)
// to an existing executable file inside the plugins directory, else E406.
func resolveBinary(dir, slug, binary string) (string, *Problem) {
	candidate := filepath.Join(dir, "bin", binary)
	if runtime.GOOS == "windows" {
		candidate += ".exe"
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", binaryProblem(slug, binary, "does not exist in the plugins bin dir")
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return "", binaryProblem(slug, binary, "is not a file")
	}
	if !insideDir(dir, resolved) {
		return "", binaryProblem(slug, binary, "resolves outside the plugins directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return "", binaryProblem(slug, binary, "is not executable")
	}
	return resolved, nil
}

// dropDuplicateCommands removes specs whose command another spec also claims
// (both lose, E405), keeping the reported order deterministic.
func dropDuplicateCommands(specs []Spec, problems *[]Problem) []Spec {
	owners := map[string][]string{}
	for _, s := range specs {
		owners[s.Command] = append(owners[s.Command], s.Slug)
	}
	var out []Spec
	for _, s := range specs {
		if len(owners[s.Command]) == 1 {
			out = append(out, s)
		}
	}
	var dups []string
	for cmd, o := range owners {
		if len(o) > 1 {
			sort.Strings(o)
			dups = append(dups, cmd)
		}
	}
	sort.Strings(dups)
	for _, cmd := range dups {
		o := owners[cmd]
		*problems = append(*problems, Problem{Code: "E405", Slug: strings.Join(o, ","),
			Message: fmt.Sprintf("command %q is declared by plugins %s; both are skipped", cmd, strings.Join(o, " and "))})
	}
	sort.Slice(*problems, func(i, j int) bool {
		if (*problems)[i].Slug != (*problems)[j].Slug {
			return (*problems)[i].Slug < (*problems)[j].Slug
		}
		return (*problems)[i].Code < (*problems)[j].Code
	})
	return out
}

// --- helpers ---

// insideDir reports whether path lies inside dir after both are resolved.
func insideDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// codeOf pulls the registered code out of a loader error; E106 is the
// fallback for a plain decode failure.
func codeOf(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) && ce.Code != "" {
		return ce.Code
	}
	return "E106"
}

func invalid(slug, field, msg string) *Problem {
	return &Problem{Code: "E106", Slug: slug, Message: "[E106] " + field + ": " + msg}
}

func collision(slug, msg string) *Problem {
	return &Problem{Code: "E405", Slug: slug, Message: "[E405] " + msg}
}

func binaryProblem(slug, binary, msg string) *Problem {
	return &Problem{Code: "E406", Slug: slug,
		Message: fmt.Sprintf("[E406] plugin %q: binary %s %s", slug, binary, msg)}
}

// rawConfirm marshals a confirm declaration to compact JSON; nil when absent.
func rawConfirm(c *manifestv2.Confirm) json.RawMessage {
	if c == nil {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil
	}
	return b
}

// subs maps manifest subcommands onto mount Subs.
func subs(list []manifestv2.Subcommand) []Sub {
	var out []Sub
	for _, s := range list {
		var args, flags json.RawMessage
		if len(s.Args) > 0 {
			args, _ = json.Marshal(s.Args)
		}
		if len(s.Flags) > 0 {
			flags, _ = json.Marshal(s.Flags)
		}
		out = append(out, Sub{
			Path: strings.Fields(s.Name), Summary: s.Summary,
			SideEffect: s.SideEffect, Output: s.Output, JSON: s.JSON,
			Args: args, Flags: flags, Confirm: rawConfirm(s.Confirm), Surface: s.Surface,
		})
	}
	return out
}
