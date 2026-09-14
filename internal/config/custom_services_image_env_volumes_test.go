package config

import "testing"

// Purpose: parse-time coverage for the three CS_N_* additions that close
// G-013 (nself build cannot express a pinned image digest, injected SMTP
// env vars, or a volume mount) — CS_N_IMAGE, CS_N_ENV_FILE, CS_N_VOLUMES.
// Mirrors the existing CS_N_PATH tests in parse_services_test.go.
// Inputs: environment variables set via t.Setenv per test.
// Outputs: none (t.Fatal/t.Error on unexpected parseCustomServices results).
// Constraints: every test clears CS_2..CS_10 so slots don't leak state.

func clearOtherCSSlots(t *testing.T, keep int) {
	t.Helper()
	for i := 1; i <= 10; i++ {
		if i == keep {
			continue
		}
		t.Setenv("CS_"+itoa(i), "")
	}
}

// TestCustomServicesImage_Valid verifies CS_N_IMAGE (with a digest suffix)
// is parsed through untouched — this is the exact shape needed to express a
// pinned minio image (G-013 evidence row 1).
func TestCustomServicesImage_Valid(t *testing.T) {
	t.Setenv("CS_1", "email-storage:go")
	t.Setenv("CS_1_IMAGE", "quay.io/minio/minio:RELEASE.2024-01-16T16-07-38Z@sha256:abc123")
	clearOtherCSSlots(t, 1)

	services, err := parseCustomServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("expected at least one custom service")
	}
	want := "quay.io/minio/minio:RELEASE.2024-01-16T16-07-38Z@sha256:abc123"
	if got := services[0].Image; got != want {
		t.Errorf("Image = %q, want %q", got, want)
	}
}

// TestCustomServicesImage_ConflictsWithPath verifies CS_N_IMAGE and CS_N_PATH
// together is rejected — a service either builds or pulls, never both.
func TestCustomServicesImage_ConflictsWithPath(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_IMAGE", "myorg/myimage:latest")
	t.Setenv("CS_1_PATH", "./services/myservice")
	clearOtherCSSlots(t, 1)

	if _, err := parseCustomServices(); err == nil {
		t.Fatal("expected error when CS_1_IMAGE and CS_1_PATH are both set")
	}
}

// TestCustomServicesEnvFile_Valid verifies CS_N_ENV_FILE accepts a clean
// relative path.
func TestCustomServicesEnvFile_Valid(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_ENV_FILE", "./secrets/smtp.env")
	clearOtherCSSlots(t, 1)

	services, err := parseCustomServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("expected at least one custom service")
	}
	if got := services[0].EnvFile; got != "./secrets/smtp.env" {
		t.Errorf("EnvFile = %q, want %q", got, "./secrets/smtp.env")
	}
}

// TestCustomServicesEnvFile_AbsoluteRejected verifies an absolute
// CS_N_ENV_FILE path is rejected, same as CS_N_PATH.
func TestCustomServicesEnvFile_AbsoluteRejected(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_ENV_FILE", "/etc/secrets/smtp.env")
	clearOtherCSSlots(t, 1)

	if _, err := parseCustomServices(); err == nil {
		t.Fatal("expected error for absolute CS_1_ENV_FILE, got nil")
	}
}

// TestCustomServicesEnvFile_TraversalRejected verifies a CS_N_ENV_FILE
// containing ".." is rejected.
func TestCustomServicesEnvFile_TraversalRejected(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_ENV_FILE", "../../outside/smtp.env")
	clearOtherCSSlots(t, 1)

	if _, err := parseCustomServices(); err == nil {
		t.Fatal("expected error for traversal CS_1_ENV_FILE, got nil")
	}
}

// TestCustomServicesVolumes_Valid verifies CS_N_VOLUMES parses a
// comma-separated list, covering the ntask email-templates mount
// (G-013 evidence row 3).
func TestCustomServicesVolumes_Valid(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_VOLUMES", "./email-templates:/app/templates:ro,my_data:/data")
	clearOtherCSSlots(t, 1)

	services, err := parseCustomServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("expected at least one custom service")
	}
	want := "./email-templates:/app/templates:ro,my_data:/data"
	if got := services[0].Volumes; got != want {
		t.Errorf("Volumes = %q, want %q", got, want)
	}
}

// TestCustomServicesVolumes_TraversalRejected verifies a relative host path
// containing ".." inside CS_N_VOLUMES is rejected.
func TestCustomServicesVolumes_TraversalRejected(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_VOLUMES", "../../outside/templates:/app/templates")
	clearOtherCSSlots(t, 1)

	if _, err := parseCustomServices(); err == nil {
		t.Fatal("expected error for traversal host path in CS_1_VOLUMES, got nil")
	}
}

// TestCustomServicesVolumes_MissingContainerPathRejected verifies an entry
// without a container path (no ":") is rejected.
func TestCustomServicesVolumes_MissingContainerPathRejected(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_VOLUMES", "./just-a-host-path")
	clearOtherCSSlots(t, 1)

	if _, err := parseCustomServices(); err == nil {
		t.Fatal("expected error for CS_1_VOLUMES entry missing a container path")
	}
}

// TestCustomServicesVolumes_AbsoluteHostAllowed verifies an absolute host
// bind mount is accepted (permissive by design, per validateCustomServiceVolumes).
func TestCustomServicesVolumes_AbsoluteHostAllowed(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_VOLUMES", "/srv/shared-templates:/app/templates:ro")
	clearOtherCSSlots(t, 1)

	services, err := parseCustomServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) == 0 || services[0].Volumes == "" {
		t.Fatal("expected CS_1_VOLUMES to be accepted")
	}
}
