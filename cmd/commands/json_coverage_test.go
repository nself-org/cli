package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"gopkg.in/yaml.v3"
)

// assertFragmentEnvelopeCoverage is called by each conversion ticket with its
// canon fragment name. Every runnable core/subcommand document row needs the
// v1.5 envelope registry class.
func assertFragmentEnvelopeCoverage(t *testing.T, fragment string) {
	t.Helper()
	path := filepath.Join("../../internal/canon/domains", fragment+".yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]cmdregistry.Command{}
	for _, row := range reg.Commands {
		byPath[row.Path] = row
	}
	for _, finding := range envelopeCoverageFindings(b, byPath) {
		t.Error(finding)
	}
}

// envelopeCoverageFindings checks a fragment with a supplied registry so the
// fixture test can prove both rejection and a reasoned reclassification.
func envelopeCoverageFindings(b []byte, byPath map[string]cmdregistry.Command) []string {
	var fragment canon.Fragment
	if err := yaml.Unmarshal(b, &fragment); err != nil {
		return []string{err.Error()}
	}
	var findings []string
	for key, row := range fragment.Commands {
		entry, ok := byPath[key]
		if !ok {
			findings = append(findings, fmt.Sprintf("%s: missing registry row", key))
			continue
		}
		if !entry.Runnable || (entry.Canon != canon.CanonCore && entry.Canon != canon.CanonSubcommand) {
			continue
		}
		output := row.Output
		if output == "" {
			output = canon.OutputDocument
		}
		if output == canon.OutputDocument {
			if entry.JSON != canon.JSONEnvelope {
				findings = append(findings, fmt.Sprintf("%s: document is json %s, want envelope", key, entry.JSON))
			}
		} else if output == canon.OutputStream || output == canon.OutputInteractive {
			if !rowHasReason(string(b), key) {
				findings = append(findings, fmt.Sprintf("%s: %s reclassification needs a reason comment on its row", key, output))
			}
		}
	}
	return findings
}

func rowHasReason(doc, key string) bool {
	for _, line := range strings.Split(doc, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, key+":") {
			return strings.Contains(trim, "#") && strings.TrimSpace(strings.SplitN(trim, "#", 2)[1]) != ""
		}
	}
	return false
}

func TestEnvelopeCoverageHelper(t *testing.T) {
	rows := map[string]cmdregistry.Command{"sample": {Path: "sample", Runnable: true, Canon: canon.CanonSubcommand, JSON: canon.JSONNone}}
	bad := []byte("schema_version: 1\ncommands:\n  sample: {side_effect: read}\n")
	if len(envelopeCoverageFindings(bad, rows)) != 1 {
		t.Fatal("document with json none was accepted")
	}
	good := []byte("schema_version: 1\ncommands:\n  sample: {side_effect: read, output: interactive} # requires a TTY\n")
	if got := envelopeCoverageFindings(good, rows); len(got) != 0 {
		t.Fatalf("reasoned interactive row rejected: %v", got)
	}
	noReason := []byte("schema_version: 1\ncommands:\n  sample: {side_effect: read, output: interactive}\n")
	if len(envelopeCoverageFindings(noReason, rows)) != 1 {
		t.Fatal("unreasoned reclassification was accepted")
	}
}
