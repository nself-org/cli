package license

import (
	"os"
	"testing"
	"time"
)

func TestSimulateOffline_GuardRequired(t *testing.T) {
	_ = os.Unsetenv(SimulationGuardEnv)
	_, err := SimulateOffline(7)
	if err == nil {
		t.Fatal("expected error when simulation guard is not set")
	}
	if err != ErrSimulationNotAllowed {
		t.Fatalf("expected ErrSimulationNotAllowed, got: %v", err)
	}
}

func TestSimulateOffline_NegativeDays(t *testing.T) {
	t.Setenv(SimulationGuardEnv, "true")
	_, err := SimulateOffline(-1)
	if err == nil {
		t.Fatal("expected error for negative days")
	}
}

func TestSimulateOffline_NoCacheReturnsError(t *testing.T) {
	t.Setenv(SimulationGuardEnv, "true")
	// Point cache to a temp dir with no cache file.
	tmpDir := t.TempDir()
	t.Setenv("LICENSE_CACHE_PATH", tmpDir+"/nonexistent.json")

	_, err := SimulateOffline(7)
	if err == nil {
		t.Fatal("expected error when no cache exists")
	}
}

func TestSimulateOffline_SetsGraceState(t *testing.T) {
	t.Setenv(SimulationGuardEnv, "true")
	tmpFile := t.TempDir() + "/license.json"
	t.Setenv("LICENSE_CACHE_PATH", tmpFile)

	// Write a fresh cache entry.
	entry := &CacheEntry{
		KeyHash:   "test_hash",
		Tier:      "pro",
		FetchedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour).Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("writing test cache: %v", err)
	}

	// Simulate 4 days offline — should be grace_soft (72h-7d window).
	result, err := SimulateOffline(4)
	if err != nil {
		t.Fatalf("SimulateOffline(4): %v", err)
	}
	if result.State != GraceSoft {
		t.Errorf("expected grace_soft after 4 days, got %s", result.State)
	}

	// Simulate 10 days offline — should be grace_hard.
	result, err = SimulateOffline(10)
	if err != nil {
		t.Fatalf("SimulateOffline(10): %v", err)
	}
	if result.State != GraceHard {
		t.Errorf("expected grace_hard after 10 days, got %s", result.State)
	}
}

func TestClearSimulation(t *testing.T) {
	t.Setenv(SimulationGuardEnv, "true")
	tmpFile := t.TempDir() + "/license.json"
	t.Setenv("LICENSE_CACHE_PATH", tmpFile)

	entry := &CacheEntry{
		KeyHash:   "test_hash",
		Tier:      "pro",
		FetchedAt: time.Now().Add(-10 * 24 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour).Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("writing test cache: %v", err)
	}

	if err := ClearSimulation(); err != nil {
		t.Fatalf("ClearSimulation: %v", err)
	}

	// After clearing, cache should be fresh (valid state).
	updated, err := ReadCache()
	if err != nil {
		t.Fatalf("reading cache after clear: %v", err)
	}
	age := time.Since(time.Unix(updated.FetchedAt, 0))
	if age > 5*time.Second {
		t.Errorf("cache age after clear should be near zero, got %v", age)
	}
}
