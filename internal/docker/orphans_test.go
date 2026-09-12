package docker

// Tests for G-014 orphan detection. No live docker daemon or real project
// containers required: buildOrphanPsArgs and parseOrphanPsOutput are pure
// functions exercised directly with canned `docker ps` output, and
// ComposeServiceNames is exercised against temp YAML files on disk.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOrphanPsArgs_ScopesToProjectLabel(t *testing.T) {
	args := buildOrphanPsArgs("myproj")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "label=com.docker.compose.project=myproj") {
		t.Errorf("args %q must filter on the project label, so unrelated containers on the host are never selected", joined)
	}
	if !strings.Contains(joined, `-a`) {
		t.Errorf("args %q must include -a so stopped orphans (e.g. a dead-since-last-month container) are found too", joined)
	}
	if !strings.Contains(joined, "com.docker.compose.service") {
		t.Errorf("args %q must request the service label so orphan detection can compare it", joined)
	}
}

func TestParseOrphanPsOutput_FlagsUndefinedServices(t *testing.T) {
	// Mirrors the measured prod incident: nself-claw/notify/mux/cron have a
	// project label but no matching service in the freshly generated compose.
	raw := strings.Join([]string{
		"abc123\tnself_postgres\tpostgres\trunning",
		"def456\tnself-claw\tclaw\trunning",
		"ghi789\tnself-mux\tmux\texited",
	}, "\n")
	defined := map[string]struct{}{"postgres": {}, "hasura": {}}

	orphans := parseOrphanPsOutput(raw, defined)
	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans, got %d: %+v", len(orphans), orphans)
	}
	names := map[string]bool{orphans[0].Name: true, orphans[1].Name: true}
	if !names["nself-claw"] || !names["nself-mux"] {
		t.Errorf("expected nself-claw and nself-mux flagged as orphans, got %+v", orphans)
	}
	for _, o := range orphans {
		if o.Service == "postgres" {
			t.Errorf("postgres has a matching service definition and must never be flagged: %+v", o)
		}
	}
}

func TestParseOrphanPsOutput_NoOrphansWhenAllDefined(t *testing.T) {
	raw := "abc123\tnself_postgres\tpostgres\trunning\ndef456\tnself_hasura\thasura\trunning"
	defined := map[string]struct{}{"postgres": {}, "hasura": {}}

	orphans := parseOrphanPsOutput(raw, defined)
	if len(orphans) != 0 {
		t.Errorf("expected no orphans when every container's service is defined, got %+v", orphans)
	}
}

func TestParseOrphanPsOutput_BlankServiceLabelIsFlagged(t *testing.T) {
	// A container with a compose-project label but no service label is
	// unexpected; report it rather than silently skip it.
	raw := "abc123\tweird-container\t\trunning"
	orphans := parseOrphanPsOutput(raw, map[string]struct{}{"postgres": {}})
	if len(orphans) != 1 {
		t.Fatalf("expected the blank-service container to be flagged, got %d orphans", len(orphans))
	}
}

func TestParseOrphanPsOutput_EmptyAndMalformedLinesSkipped(t *testing.T) {
	raw := "\n\nabc123\tonlytwo\tfields\n"
	orphans := parseOrphanPsOutput(raw, map[string]struct{}{})
	if len(orphans) != 0 {
		t.Errorf("malformed (too few columns) and blank lines must be skipped, got %+v", orphans)
	}
}

func TestComposeServiceNames_UnionsMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "docker-compose.yml")
	plugin := filepath.Join(dir, "plugin-claw.yml")

	if err := os.WriteFile(base, []byte("name: myproj\nservices:\n  postgres:\n    image: postgres:16\n  hasura:\n    image: hasura/graphql-engine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plugin, []byte("services:\n  claw:\n    image: nself/claw:latest\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	names, err := ComposeServiceNames([]string{base, plugin})
	if err != nil {
		t.Fatalf("ComposeServiceNames returned error: %v", err)
	}
	for _, want := range []string{"postgres", "hasura", "claw"} {
		if _, ok := names[want]; !ok {
			t.Errorf("expected service %q in union, got %+v", want, names)
		}
	}
	if len(names) != 3 {
		t.Errorf("expected exactly 3 services, got %d: %+v", len(names), names)
	}
}

func TestComposeServiceNames_MissingFileSkippedNotFatal(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(base, []byte("services:\n  postgres:\n    image: postgres:16\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "removed-plugin.yml")

	names, err := ComposeServiceNames([]string{base, missing})
	if err != nil {
		t.Fatalf("a missing compose fragment must not be a hard error, got: %v", err)
	}
	if _, ok := names["postgres"]; !ok {
		t.Errorf("expected postgres from the readable file, got %+v", names)
	}
}

func TestComposeServiceNames_InvalidYAMLIsHardError(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(bad, []byte(":\n  - not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ComposeServiceNames([]string{bad}); err == nil {
		t.Error("expected an error for a file that exists but fails to parse as YAML")
	}
}
