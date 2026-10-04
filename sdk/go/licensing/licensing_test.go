package licensing

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestHashKeyStable(t *testing.T) {
	if HashKey("abc") != HashKey("abc") {
		t.Errorf("hash should be stable")
	}
	if HashKey("a") == HashKey("b") {
		t.Errorf("different inputs should hash differently")
	}
}

func TestCacheFresh(t *testing.T) {
	now := time.Now()
	c := CachedValidation{ValidatedAt: now.Add(-24 * time.Hour)}
	if !c.Fresh(now, 7*24*time.Hour) {
		t.Errorf("1 day old should be fresh with 7d grace")
	}
	if c.Fresh(now, time.Hour) {
		t.Errorf("1 day old should NOT be fresh with 1h grace")
	}
	c2 := CachedValidation{ValidatedAt: now, ExpiresAt: now.Add(-time.Hour)}
	if c2.Fresh(now, 7*24*time.Hour) {
		t.Errorf("expired cache should not be fresh regardless of grace")
	}
}

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()
	v := NewValidator(filepath.Join(dir, "cache.json"))
	c := CachedValidation{
		KeyHash:     HashKey("nself_pro_abc123"),
		Tier:        "basic",
		Plugins:     []string{"notify", "cron"},
		ValidatedAt: time.Now(),
	}
	if err := v.Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(v.CachePath)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("perms %o, want 0600", info.Mode().Perm())
		}
	}
	loaded, err := v.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Tier != "basic" || len(loaded.Plugins) != 2 {
		t.Errorf("loaded cache mismatch: %+v", loaded)
	}
}

func TestRemainingGrace(t *testing.T) {
	now := time.Now()
	c := CachedValidation{ValidatedAt: now.Add(-2 * 24 * time.Hour)}
	left := c.RemainingGrace(now, 7*24*time.Hour)
	if left <= 0 || left > 5*24*time.Hour+time.Minute {
		t.Errorf("expected ~5 days remaining, got %s", left)
	}
	stale := CachedValidation{ValidatedAt: now.Add(-30 * 24 * time.Hour)}
	if stale.RemainingGrace(now, 7*24*time.Hour) != 0 {
		t.Errorf("stale cache should have 0 grace remaining")
	}
	expired := CachedValidation{ValidatedAt: now, ExpiresAt: now.Add(-time.Hour)}
	if expired.RemainingGrace(now, 7*24*time.Hour) != 0 {
		t.Errorf("expired cache should have 0 grace remaining")
	}
}

func TestDefaultCachePath(t *testing.T) {
	t.Setenv("NSELF_LICENSE_CACHE", "/tmp/test-override.json")
	if DefaultCachePath() != "/tmp/test-override.json" {
		t.Errorf("env override ignored")
	}
	t.Setenv("NSELF_LICENSE_CACHE", "")
	p := DefaultCachePath()
	if p == "" {
		t.Errorf("default path should not be empty")
	}
	if !strings.Contains(p, ".nself") {
		t.Errorf("default path should contain .nself, got %q", p)
	}
}

func TestAllowPlugin(t *testing.T) {
	dir := t.TempDir()
	v := NewValidator(filepath.Join(dir, "cache.json"))
	now := time.Now()

	// No cache → error.
	if err := v.AllowPlugin("notify", "", now); err == nil {
		t.Errorf("expected error with empty cache")
	}

	// Fresh cache including plugin → allow.
	_ = v.Save(CachedValidation{
		KeyHash:     HashKey("key"),
		Tier:        "basic",
		Plugins:     []string{"notify"},
		ValidatedAt: now,
	})
	if err := v.AllowPlugin("notify", "key", now); err != nil {
		t.Errorf("allow should succeed: %v", err)
	}
	// Wrong plugin.
	if err := v.AllowPlugin("ai", "key", now); err == nil {
		t.Errorf("ai should not be allowed with basic tier")
	}
	// Stale.
	_ = v.Save(CachedValidation{
		KeyHash:     HashKey("key"),
		Tier:        "basic",
		Plugins:     []string{"notify"},
		ValidatedAt: now.Add(-30 * 24 * time.Hour),
	})
	if err := v.AllowPlugin("notify", "key", now); err == nil {
		t.Errorf("30d old cache should be stale")
	}

	// SkipVerify overrides.
	v.SkipVerify = true
	if err := v.AllowPlugin("whatever", "", now); err != nil {
		t.Errorf("SkipVerify should allow: %v", err)
	}
}
