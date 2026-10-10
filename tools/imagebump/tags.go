package main

// tags.go — where candidate tags come from.
//
// Purpose: give planBumps one way to list the tags of a repository.
// Inputs: a repository such as docker.io/prom/prometheus.
// Outputs: the tag names, unordered.
// Constraints: production reads go through scripts/images/mirror-images.sh
// --list-tags (crane in a digest-pinned container); no Go registry client
// (Constitution §12). Unit tests use fixtureLister and never touch a network.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// TagLister lists the tags of one repository.
type TagLister interface {
	Tags(repository string) ([]string, error)
}

// defaultListTimeout bounds one repository read. crane pages the list, and
// docker.elastic.co/elasticsearch/elasticsearch (about 49000 tags) needs
// roughly 15 minutes; every other locked repository takes seconds.
const defaultListTimeout = 30 * time.Minute

// listTimeout returns IMAGEBUMP_LIST_TIMEOUT (a Go duration) or the default.
func listTimeout() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("IMAGEBUMP_LIST_TIMEOUT")); err == nil && d > 0 {
		return d
	}
	return defaultListTimeout
}

// execLister shells out to the mirror tool.
type execLister struct{ script string }

// Tags runs `bash <script> --list-tags <repository>` and returns its lines.
func (l *execLister) Tags(repository string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", l.script, "--list-tags", repository)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list tags %s: %w: %s", repository, err, strings.TrimSpace(stderr.String()))
	}
	return splitLines(stdout.String()), nil
}

// splitLines returns the non-empty trimmed lines of s.
func splitLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

// fixtureLister serves tags from the --lock file's fixture_tags map.
type fixtureLister map[string][]string

// Tags returns the fixture tags of repository; a repository without an entry is
// an error so a typo in a fixture cannot pass silently.
func (f fixtureLister) Tags(repository string) ([]string, error) {
	t, ok := f[repository]
	if !ok {
		return nil, fmt.Errorf("fixture_tags has no entry for %s", repository)
	}
	return t, nil
}

// cachedLister asks the inner lister once per repository (several entries can
// share one, e.g. the two otel collector entries).
type cachedLister struct {
	inner TagLister
	seen  map[string][]string
}

func newCachedLister(inner TagLister) *cachedLister {
	return &cachedLister{inner: inner, seen: map[string][]string{}}
}

// Tags returns the cached tags, or reads them through the inner lister.
func (c *cachedLister) Tags(repository string) ([]string, error) {
	if t, ok := c.seen[repository]; ok {
		return t, nil
	}
	t, err := c.inner.Tags(repository)
	if err != nil {
		return nil, err
	}
	c.seen[repository] = t
	return t, nil
}
