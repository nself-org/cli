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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SourceFile is one compose file's bytes. Name is shown in messages.
type SourceFile struct {
	Name   string
	Data   []byte
	Mirror bool
}

// Merged is the result of MergeDetailed.
type Merged struct {
	// Images is service -> merged image reference.
	Images map[string]string
	// Unverifiable lists, sorted and without duplicates, what could not be read
	// as a plain image.
	Unverifiable []string
	// Services names the CLI-owned services behind Unverifiable (sorted); an
	// include has no service of its own, so it adds none.
	Services []string
}

// MergeImages merges the image keys of files in order. unverifiable lists, sorted
// and without duplicates, what could not be read as a plain image.
func MergeImages(files []SourceFile, owned func(service string) bool, forCompare bool) (merged map[string]string, unverifiable []string, err error) {
	m, err := MergeDetailed(files, owned, forCompare)
	return m.Images, m.Unverifiable, err
}

// MergeDetailed is MergeImages that also names the unverifiable services.
func MergeDetailed(files []SourceFile, owned func(service string) bool, forCompare bool) (Merged, error) {
	merged := map[string]string{}
	problems := map[string]bool{}
	bad := map[string]bool{}
	for _, f := range files {
		if f.Mirror && !forCompare {
			continue
		}
		svcs, include, multi, err := ComposeSources(f.Data)
		if err != nil {
			return Merged{}, fmt.Errorf("%s: %w", f.Name, err)
		}
		if multi {
			problems["more than one YAML document in "+f.Name+": image source cannot be verified"] = true
		}
		if include {
			problems["include in "+f.Name+": image source cannot be verified"] = true
		}
		for name, s := range svcs {
			if owned(name) {
				switch {
				case s.Extends:
					problems[name+": extends in "+f.Name+": image source cannot be verified"], bad[name] = true, true
				case s.Build:
					problems[name+": build in "+f.Name+": image source cannot be verified"], bad[name] = true, true
				case strings.Contains(s.Image, "$"):
					problems[name+": the image "+s.Image+" in "+f.Name+" contains a variable: image source cannot be verified"], bad[name] = true, true
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
	out := Merged{Images: merged}
	for p := range problems {
		out.Unverifiable = append(out.Unverifiable, p)
	}
	for n := range bad {
		out.Services = append(out.Services, n)
	}
	sort.Strings(out.Unverifiable)
	sort.Strings(out.Services)
	return out, nil
}

// LoadSources reads the compose files start runs, in the order start runs
// them: paths exactly as given (relative ones resolve against workdir), so the
// merge order (last wins) is the one `docker compose -f ...` sees. An empty
// paths means workdir's docker-compose.yml alone. A non-empty paths that does
// not list docker-compose.yml is an error: start would run a stack without the
// base file. base replaces the bytes of docker-compose.yml when non-nil. A
// listed file that does not exist is skipped, except the base. mirror is the
// project-relative path of the registry-mirror override file; its SourceFile
// has Mirror set.
func LoadSources(workdir string, base []byte, paths []string, mirror string) ([]SourceFile, error) {
	abs := func(p string) string {
		if !filepath.IsAbs(p) {
			p = filepath.Join(workdir, p)
		}
		return filepath.Clean(p)
	}
	basePath, mirrorPath := abs("docker-compose.yml"), abs(mirror)
	if len(paths) == 0 {
		paths = []string{basePath}
	}
	listed := false
	for _, p := range paths {
		listed = listed || samePath(abs(p), basePath)
	}
	if !listed {
		return nil, errors.New("compose manifest does not include docker-compose.yml")
	}
	var files []SourceFile
	for _, p := range paths {
		p = abs(p)
		data := base
		if !samePath(p, basePath) || base == nil {
			var err error
			if data, err = os.ReadFile(p); err != nil {
				if !samePath(p, basePath) && os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("reading %s: %w", filepath.Base(p), err)
			}
		}
		files = append(files, SourceFile{Name: filepath.Base(p), Data: data, Mirror: samePath(p, mirrorPath)})
	}
	return files, nil
}

// samePath reports whether a and b name the same file. Equal cleaned strings
// match; otherwise both are stat'd and compared with os.SameFile, so a
// symlinked or differently spelled project dir still matches. When either stat
// fails (the file may not exist) the string comparison stands.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(sa, sb)
}
