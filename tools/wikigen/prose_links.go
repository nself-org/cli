package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var proseURL = regexp.MustCompile(`https?://[^\s<>\x60)\]]+`)
var proseLinkTarget = regexp.MustCompile(`(?m)(?:\]\(\s*|^\[[^\]]+\]:\s*)<?([^)\s>]+)`)

// validateProseLinks checks public command links against the wiki sources that
// feed the documentation site. Private planning paths never belong in links.
func validateProseLinks(dir, body string) error {
	targets := proseURL.FindAllString(body, -1)
	for _, link := range proseLinkTarget.FindAllStringSubmatch(body, -1) {
		targets = append(targets, link[1])
	}
	for _, raw := range targets {
		u, err := url.Parse(raw)
		if err != nil {
			// Other hosts may deliberately use placeholders such as MINIO_PORT.
			if !strings.Contains(raw, ".claude") && !strings.HasPrefix(raw, "https://nself.org/docs") && !strings.HasPrefix(raw, "http://nself.org/docs") {
				continue
			}
			return fmt.Errorf("invalid documentation URL: %w", err)
		}
		for _, part := range strings.Split(u.Path, "/") {
			if part == ".claude" {
				return fmt.Errorf("public prose links to private planning path: %s", raw)
			}
		}
		if u.Host != "nself.org" || (u.Path != "/docs" && !strings.HasPrefix(u.Path, "/docs/")) {
			continue
		}
		route := strings.Trim(strings.TrimPrefix(u.Path, "/docs"), "/")
		if route == "" {
			continue
		}
		if strings.HasPrefix(route, "commands/") {
			route = "cmd-" + strings.TrimPrefix(route, "commands/")
		}
		if strings.Contains(route, "..") {
			return fmt.Errorf("invalid documentation route: %s", raw)
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(route)+".md")); err != nil {
			return fmt.Errorf("documentation link has no wiki source: %s", raw)
		}
	}
	return nil
}

func checkCommandProseLinks(dir string) error {
	pages, err := filepath.Glob(filepath.Join(dir, "cmd-*.md"))
	if err != nil {
		return err
	}
	for _, page := range pages {
		body, err := os.ReadFile(page)
		if err != nil {
			return err
		}
		if err := validateProseLinks(dir, string(body)); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(page), err)
		}
	}
	return nil
}
