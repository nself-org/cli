package lock

// merge.go — the image of every service across the compose files `start` runs.
//
// Purpose: `nself start` passes the generated compose, plugin fragments and
// docker-compose.override.yml to docker compose with -f; the last file that
// sets a service's image wins. The lock records, and `start --frozen`
// compares, that merged result for the services the CLI owns.
// Inputs: the files in start's order (already read) and a predicate for the
// CLI-owned service names.
// Outputs: service -> image, and the reasons an image cannot be verified.
// Constraints: pure. An entry of the generated registry-mirror override file
// (SourceFile.Mirror) is never recorded (forCompare false); when comparing
// (forCompare true) it counts only if its digest equals the digest of the
// reference it replaces, so a mirror serving the same bytes is not a change.
// A CLI-owned service whose image is built from a variable, extends another
// service, has a build section, or any file with a top-level include, cannot
// be read without running docker compose, so it is reported, not guessed.

import (
	"fmt"
	"sort"
	"strings"
)

// SourceFile is one compose file's bytes. Name is shown in messages.
type SourceFile struct {
	Name   string
	Data   []byte
	Mirror bool
}

// MergeImages merges the image keys of files in order. unverifiable lists, sorted
// and without duplicates, what could not be read as a plain image.
func MergeImages(files []SourceFile, owned func(service string) bool, forCompare bool) (merged map[string]string, unverifiable []string, err error) {
	merged = map[string]string{}
	problems := map[string]bool{}
	for _, f := range files {
		if f.Mirror && !forCompare {
			continue
		}
		svcs, include, err := ComposeSources(f.Data)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if include {
			problems["include in "+f.Name+": image source cannot be verified"] = true
		}
		for name, s := range svcs {
			if owned(name) {
				switch {
				case s.Extends:
					problems[name+": extends in "+f.Name+": image source cannot be verified"] = true
				case s.Build:
					problems[name+": build in "+f.Name+": image source cannot be verified"] = true
				case strings.Contains(s.Image, "$"):
					problems[name+": the image "+s.Image+" in "+f.Name+" contains a variable: image source cannot be verified"] = true
				}
			}
			if s.Image == "" {
				continue
			}
			// A same-digest mirror serves the bytes the lock pins.
			if prev, seen := merged[name]; f.Mirror && seen && RefDigest(s.Image) != "" && RefDigest(s.Image) == RefDigest(prev) {
				continue
			}
			merged[name] = s.Image
		}
	}
	for p := range problems {
		unverifiable = append(unverifiable, p)
	}
	sort.Strings(unverifiable)
	return merged, unverifiable, nil
}
