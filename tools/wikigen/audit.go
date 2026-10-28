package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	claudeLinkRe  = regexp.MustCompile(`github\.com/nself-org/cli/blob/main/\.claude`)
	docsLinkRe    = regexp.MustCompile(`https://nself\.org/docs/[^\s\)]+`)
	wikiLinkRe    = regexp.MustCompile(`\[\[([^\]|#]+)`)
	inlineCodeRe  = regexp.MustCompile("`[^`]*`")
	publishedDocs = map[string]bool{
		"https://nself.org/docs/commands":  true,
		"https://nself.org/docs/plugins":   true,
		"https://nself.org/docs/guides":    true,
		"https://nself.org/docs/reference": true,
	}
)

// wikiPageSet returns the basename (no .md) of every page under dir. GitHub
// Wiki flattens the namespace, so a page in a subdirectory is reachable by its
// basename.
func wikiPageSet(dir string) (map[string]bool, error) {
	pages := map[string]bool{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			pages[strings.TrimSuffix(d.Name(), ".md")] = true
		}
		return nil
	})
	return pages, err
}

// auditProseLinks fails when a cmd-*.md page links into the private .claude
// workspace or to an nself.org/docs URL that is not in the published docs nav,
// or when any wiki page has a [[wikilink]] whose target page does not exist.
// Each class of link 404s in public renders (D-0717).
//
// Inputs: the wiki directory (searched recursively). Outputs: one message per
// problem, sorted by page. Constraints: fenced code blocks hold verbatim CLI
// output, so they are skipped for wikilinks (as scripts/ci/wiki-link-audit.sh
// does) and for docs URLs; inline `code` spans are skipped for wikilinks.
func auditProseLinks(dir string) ([]string, error) {
	pages, err := wikiPageSet(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	var bad []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(path)
		proseOnly := strings.HasPrefix(name, "cmd-")
		if proseOnly && claudeLinkRe.Match(data) {
			bad = append(bad, fmt.Sprintf("%s: contains .claude/ link", name))
		}
		inFence := false
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			for _, m := range docsLinkRe.FindAllString(line, -1) {
				if proseOnly && !publishedDocs[strings.TrimRight(m, ").,;:")] {
					bad = append(bad, fmt.Sprintf("%s: broken docs link %s", name, m))
				}
			}
			for _, m := range wikiLinkRe.FindAllStringSubmatch(inlineCodeRe.ReplaceAllString(line, ""), -1) {
				target := strings.TrimSuffix(strings.TrimSpace(m[1]), ".md")
				target = target[strings.LastIndex(target, "/")+1:]
				if target != "" && !pages[target] {
					bad = append(bad, fmt.Sprintf("%s: broken wikilink [[%s]]", name, target))
				}
			}
		}
	}
	return bad, nil
}
