package main

import (
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// docsNav is an offline snapshot of the public docs navigation. Its pinned
// sources and refresh procedure are documented in .github/docs/wiki-links.md.
//
//go:embed docs-nav.txt
var docsNav string

// publicLinks finds Markdown destinations (including reference definitions)
// and bare/autolink URLs. Plain code paths such as `.claude/settings.json`
// are examples, not links, and remain valid documentation.
var publicLinks = regexp.MustCompile(`(?m)\]\(\s*<?([^\s)>]+)|^\s{0,3}\[[^\]\n]+\]:\s*<?([^\s>]+)|(?:https?:)?//[^\s<>` + "`" + `)]+`)

// checkWikiLinks validates all command pages, including legacy plugin pages
// without full PROSE markers. It runs before generation so failures never
// leave partially rewritten pages. Inputs: wiki directory. Output: diagnostics.
func checkWikiLinks(dir string) error {
	pages, err := filepath.Glob(filepath.Join(dir, "cmd-*.md"))
	if err != nil {
		return err
	}
	nav := map[string]bool{"": true}
	for _, slug := range strings.Split(docsNav, "\n") {
		if slug != "" && !strings.HasPrefix(slug, "#") {
			nav[slug] = true
		}
	}
	var problems []string
	for _, page := range pages {
		body, err := os.ReadFile(page)
		if err != nil {
			return err
		}
		for _, problem := range invalidPublicLinks(string(body), nav) {
			problems = append(problems, filepath.Base(page)+": "+problem)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid wiki command links:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

// invalidPublicLinks checks link destinations against public path boundaries
// and the docs navigation; fragments and queries do not change page identity.
func invalidPublicLinks(body string, nav map[string]bool) []string {
	var problems []string
	for _, match := range publicLinks.FindAllStringSubmatch(body, -1) {
		dest := match[0]
		for _, group := range match[1:] {
			if group != "" {
				dest = group
				break
			}
		}
		dest = strings.TrimRight(dest, ".,;:")
		u, err := url.Parse(dest)
		if err != nil {
			// Unrelated example URLs may contain placeholders such as MINIO_PORT.
			if strings.Contains(dest, "nself.org/docs") || strings.Contains(dest, ".claude") {
				problems = append(problems, "invalid link "+dest)
			}
			continue
		}
		private := false
		for _, segment := range strings.Split(u.Path, "/") {
			private = private || segment == ".claude"
		}
		if private {
			problems = append(problems, "private workspace link "+dest)
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host != "nself.org" && host != "www.nself.org" && (host != "" || !strings.HasPrefix(u.Path, "/docs")) {
			continue
		}
		clean := path.Clean(u.Path)
		if clean != "/docs" && !strings.HasPrefix(clean, "/docs/") {
			continue
		}
		slug := strings.TrimPrefix(strings.TrimPrefix(clean, "/docs"), "/")
		if !nav[slug] {
			problems = append(problems, "docs page missing from navigation: "+dest)
		}
	}
	return problems
}
