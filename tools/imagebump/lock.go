package main

// lock.go — reading and rewriting the authored image list.
//
// Purpose: parse images.yaml (plus the fixture-only fixture_tags key), plan the
// bumps, and rewrite only the changed `version` values.
// Inputs: images.yaml bytes, the Policy, a TagLister.
// Outputs: []Bump sorted by name; the rewritten bytes.
// Constraints: the rewrite is a text edit of one-line flow entries
// (`- {name: x, ..., version: v, ...}`); every other byte stays, the existing
// quoting style is kept, and a new value YAML would not read as a string (4.49)
// is double-quoted. The result is parsed again and every version is checked.

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Image is the part of an images.yaml entry imagebump needs.
type Image struct {
	Name       string `yaml:"name"`
	Role       string `yaml:"role"`
	Repository string `yaml:"repository"`
	Version    string `yaml:"version"`
}

// lockFile is a parsed list. Unknown keys are ignored here: imagelock decodes
// the real file strictly. FixtureTags is non-nil only in test fixtures.
type lockFile struct {
	Images      []Image             `yaml:"images"`
	FixtureTags map[string][]string `yaml:"fixture_tags"`
}

// Bump is one proposed pin move.
type Bump struct{ Name, Old, New string }

func parseLockFile(raw []byte) (*lockFile, error) {
	var lf lockFile
	if err := yaml.Unmarshal(raw, &lf); err != nil {
		return nil, err
	}
	return &lf, nil
}

// planBumps applies the policy to every non-plugin image. Every such image must
// have a policy entry whose regex matches its current version; all violations
// are returned together. Policy entries with no matching image are noted on
// note. Bumps are sorted by name.
func planBumps(images []Image, pol *Policy, lister TagLister, note io.Writer) ([]Bump, []error) {
	var bumps []Bump
	var problems []error
	known := map[string]bool{}
	for _, img := range images {
		known[img.Name] = true
		if img.Role == "plugin" {
			continue
		}
		r, ok := pol.rules[img.Name]
		if !ok {
			problems = append(problems, fmt.Errorf("%s: no entry in the bump policy", img.Name))
			continue
		}
		if r.track == "manual" {
			say(note, "imagebump: %s: track manual, skipped\n", img.Name)
			continue
		}
		tags, err := lister.Tags(img.Repository)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", img.Name, err))
			continue
		}
		pick, err := selectTag(r, img.Version, tags)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", img.Name, err))
			continue
		}
		if pick != img.Version {
			bumps = append(bumps, Bump{Name: img.Name, Old: img.Version, New: pick})
		}
	}
	for _, n := range pol.names() {
		if !known[n] {
			say(note, "imagebump: policy entry %s has no image in the list\n", n)
		}
	}
	sort.Slice(bumps, func(i, j int) bool { return bumps[i].Name < bumps[j].Name })
	return bumps, problems
}

var versionKeyRe = regexp.MustCompile(`((?:\{|,)\s*version:\s*)("[^"]*"|'[^']*'|[^,}\s]+)`)

// entryLineRe returns a regexp matching the flow entry line of name.
func entryLineRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?:\{|,)\s*name:\s*["']?` + regexp.QuoteMeta(name) + `["']?\s*(?:,|\})`)
}

// renderVersion writes nv in the quoting style of old; an unquoted old value
// stays unquoted unless YAML would not read nv back as a string.
func renderVersion(old, nv string) string {
	switch old[0] {
	case '"':
		return `"` + nv + `"`
	case '\'':
		return "'" + nv + "'"
	}
	var v any
	plain := strings.Trim(nv, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-") == ""
	if plain && yaml.Unmarshal([]byte(nv), &v) == nil {
		if _, isString := v.(string); isString {
			return nv
		}
	}
	return `"` + nv + `"`
}

// rewriteVersions returns raw with the version of each bumped entry replaced.
// It fails when an entry line is not found exactly once or the result does not
// parse back to the expected versions.
func rewriteVersions(raw []byte, bumps []Bump) ([]byte, error) {
	lines := strings.SplitAfter(string(raw), "\n")
	for _, b := range bumps {
		re := entryLineRe(b.Name)
		idx := -1
		for i, ln := range lines {
			if re.MatchString(ln) {
				if idx >= 0 {
					return nil, fmt.Errorf("rewrite: entry %s appears on more than one line", b.Name)
				}
				idx = i
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("rewrite: no one-line flow entry for %s", b.Name)
		}
		loc := versionKeyRe.FindStringSubmatchIndex(lines[idx])
		if loc == nil {
			return nil, fmt.Errorf("rewrite: no version key on the line of %s", b.Name)
		}
		old := lines[idx][loc[4]:loc[5]]
		lines[idx] = lines[idx][:loc[4]] + renderVersion(old, b.New) + lines[idx][loc[5]:]
	}
	out := []byte(strings.Join(lines, ""))
	return out, verifyRewrite(raw, out, bumps)
}

// verifyRewrite checks that out differs from before only by the bumped versions.
func verifyRewrite(before, after []byte, bumps []Bump) error {
	a, err := parseLockFile(before)
	if err != nil {
		return err
	}
	b, err := parseLockFile(after)
	if err != nil {
		return fmt.Errorf("rewrite produced invalid YAML: %w", err)
	}
	want := map[string]string{}
	for _, img := range a.Images {
		want[img.Name] = img.Version
	}
	for _, bu := range bumps {
		want[bu.Name] = bu.New
	}
	if len(a.Images) != len(b.Images) {
		return errors.New("rewrite changed the number of entries")
	}
	for _, img := range b.Images {
		if want[img.Name] != img.Version {
			return fmt.Errorf("rewrite: %s is %q, want %q", img.Name, img.Version, want[img.Name])
		}
	}
	return nil
}
