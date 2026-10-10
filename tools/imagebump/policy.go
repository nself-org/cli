package main

// policy.go — the bump policy and tag selection.
//
// Purpose: decide, per image, which registry tag the weekly bump may move to.
// Inputs: .github/image-bump-policy.yaml bytes; an entry's current version and
// the repository's tag list.
// Outputs: the chosen tag (the current one when nothing newer is allowed).
// Constraints: regex is anchored RE2 (^...$); its capture groups are the numeric
// version components, so a pre-release tag that the regex does not match is never
// a candidate. track patch keeps the first two groups equal to the current tag's,
// track minor keeps the first one; a tag with fewer groups than that keeps all
// its groups fixed, so it never moves. No groups (latest, alpine, pg16) means no
// version movement. The highest candidate by numeric comparison wins and is never
// lower than the current version. track manual skips the entry.

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Rule is one policy entry as authored.
type Rule struct {
	Regex string `yaml:"regex"`
	Track string `yaml:"track"`
}

// rule is a validated Rule with its regex compiled.
type rule struct {
	re    *regexp.Regexp
	track string
}

// Policy maps lock entry names to rules.
type Policy struct {
	rules map[string]rule
}

// loadPolicy parses and validates the policy (strict keys, schema_version 1,
// anchored compilable regex, known track).
func loadPolicy(data []byte) (*Policy, error) {
	var doc struct {
		SchemaVersion int             `yaml:"schema_version"`
		Services      map[string]Rule `yaml:"services"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if doc.SchemaVersion != 1 {
		return nil, fmt.Errorf("schema_version %d, want 1", doc.SchemaVersion)
	}
	p := &Policy{rules: map[string]rule{}}
	for name, r := range doc.Services {
		if r.Track != "patch" && r.Track != "minor" && r.Track != "manual" {
			return nil, fmt.Errorf("service %s: track %q, want patch|minor|manual", name, r.Track)
		}
		if !strings.HasPrefix(r.Regex, "^") || !strings.HasSuffix(r.Regex, "$") {
			return nil, fmt.Errorf("service %s: regex %q must be anchored with ^ and $", name, r.Regex)
		}
		re, err := regexp.Compile(r.Regex)
		if err != nil {
			return nil, fmt.Errorf("service %s: regex: %w", name, err)
		}
		p.rules[name] = rule{re: re, track: r.Track}
	}
	return p, nil
}

// names returns the policy's service names, sorted.
func (p *Policy) names() []string {
	out := make([]string, 0, len(p.rules))
	for n := range p.rules {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// versionOf returns the numeric groups of tag. matched is false when the regex
// does not match the whole tag. Trailing empty groups (optional parts) are
// dropped; an empty group before a filled one, or a non-number, is an error.
func versionOf(re *regexp.Regexp, tag string) (nums []int, matched bool, err error) {
	m := re.FindStringSubmatch(tag)
	if m == nil || m[0] != tag {
		return nil, false, nil
	}
	sub := m[1:]
	for len(sub) > 0 && sub[len(sub)-1] == "" {
		sub = sub[:len(sub)-1]
	}
	for _, s := range sub {
		n, convErr := strconv.Atoi(s)
		if convErr != nil {
			return nil, true, fmt.Errorf("tag %q: group %q is not a number", tag, s)
		}
		nums = append(nums, n)
	}
	return nums, true, nil
}

// compareVersions compares two component lists numerically; a missing
// component counts as 0.
func compareVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// errMismatch reports a current version outside its policy regex.
var errMismatch = fmt.Errorf("current version does not match the policy regex")

// selectTag returns the tag the rule allows given the current version and the
// repository's tags. It returns current when nothing newer qualifies, and
// errMismatch (wrapped) when current itself does not match the regex.
func selectTag(r rule, current string, tags []string) (string, error) {
	cur, ok, err := versionOf(r.re, current)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%w: %q vs %s", errMismatch, current, r.re)
	}
	fixed := 2
	if r.track == "minor" {
		fixed = 1
	}
	if fixed > len(cur) {
		fixed = len(cur)
	}
	best, bestTag := cur, current
	for _, t := range tags {
		v, matched, vErr := versionOf(r.re, t)
		if !matched || vErr != nil || len(v) < fixed || compareVersions(v[:fixed], cur[:fixed]) != 0 {
			continue
		}
		c := compareVersions(v, best)
		if c > 0 || (c == 0 && bestTag != current && t > bestTag) {
			best, bestTag = v, t
		}
	}
	return bestTag, nil
}
