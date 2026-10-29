package postgres

import (
	"strings"
	"testing"
)

// TestImageChangeWarning_NoRunningContainer verifies the advisory
// check never errors or fabricates a warning when the container doesn't
// exist (e.g. Docker isn't running, or this is a first-ever build) — a
// missing baseline must never block `nself build`.
func TestImageChangeWarning_NoRunningContainer(t *testing.T) {
	got := ImageChangeWarning("nonexistent_project_postgres_container_xyz", "pgvector/pgvector:pg16")
	if got != "" {
		t.Errorf("ImageChangeWarning(no container) = %q, want empty", got)
	}
}

// TestImageChangeWarning_Mismatch verifies cli#384's core case: a
// running pgvector container against a regen that would produce a plain
// alpine image must produce a WARNING naming both images and the fix.
func TestImageChangeWarning_Mismatch(t *testing.T) {
	lookup := func(name string) string {
		if name == "myproj_postgres" {
			return "pgvector/pgvector:pg16"
		}
		return ""
	}
	got := imageChangeWarning("myproj_postgres", "postgres:16-alpine", lookup)
	if got == "" {
		t.Fatal("expected a warning, got empty string")
	}
	for _, want := range []string{"myproj_postgres", "pgvector/pgvector:pg16", "postgres:16-alpine", "POSTGRES_IMAGE"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning %q missing expected substring %q", got, want)
		}
	}
}

// TestImageChangeWarning_Match verifies no warning is raised when the
// running container's image already matches what this build generated.
func TestImageChangeWarning_Match(t *testing.T) {
	lookup := func(string) string { return "pgvector/pgvector:pg16" }
	got := imageChangeWarning("myproj_postgres", "pgvector/pgvector:pg16", lookup)
	if got != "" {
		t.Errorf("imageChangeWarning(match) = %q, want empty", got)
	}
}

// TestImageChangeWarning_EmptyBaselineIsSilent verifies an empty
// lookup result (no running container, or Docker unreachable) is treated as
// "nothing to compare", not as a mismatch.
func TestImageChangeWarning_EmptyBaselineIsSilent(t *testing.T) {
	lookup := func(string) string { return "" }
	got := imageChangeWarning("myproj_postgres", "postgres:16-alpine", lookup)
	if got != "" {
		t.Errorf("imageChangeWarning(empty baseline) = %q, want empty", got)
	}
}
