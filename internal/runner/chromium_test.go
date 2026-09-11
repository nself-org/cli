package runner

import (
	"context"
	"strings"
	"testing"
)

func TestChromiumLddScript_SubstitutesHome(t *testing.T) {
	script := chromiumLddScript([]string{"%h/.cache/ms-playwright/chromium-*/chrome-linux/chrome"})
	if !strings.Contains(script, "$home/.cache/ms-playwright/chromium-*/chrome-linux/chrome") {
		t.Fatalf("script did not substitute %%h: %s", script)
	}
	if !strings.Contains(script, "nullglob") {
		t.Fatalf("script missing nullglob guard: %s", script)
	}
}

func TestParseChromiumLddOutput_AllLibsResolve(t *testing.T) {
	out := "CHROME /home/gha-runner/.cache/ms-playwright/chromium-123/chrome-linux/chrome"
	results := parseChromiumLddOutput(out)
	if len(results) != 1 || results[0].Status != StatusPass {
		t.Fatalf("results = %+v, want single pass", results)
	}
}

func TestParseChromiumLddOutput_MissingLib(t *testing.T) {
	out := "CHROME /home/gha-runner/.cache/ms-playwright/chromium-123/chrome-linux/chrome\n" +
		"MISSING \tlibnspr4.so => not found"
	results := parseChromiumLddOutput(out)
	if len(results) != 1 {
		t.Fatalf("results = %+v, want 1", results)
	}
	if results[0].Status != StatusFail {
		t.Fatalf("status = %v, want fail — this is the exact 2026-09-11 failure mode", results[0].Status)
	}
	if !strings.Contains(results[0].Detail, "libnspr4.so") {
		t.Fatalf("detail = %q, want it to name the missing lib", results[0].Detail)
	}
}

func TestParseChromiumLddOutput_MultipleBinaries(t *testing.T) {
	out := "CHROME /a/chrome\n" +
		"MISSING libnspr4.so => not found\n" +
		"CHROME /b/headless_shell\n"
	results := parseChromiumLddOutput(out)
	if len(results) != 2 {
		t.Fatalf("results = %+v, want 2", results)
	}
	if results[0].Status != StatusFail {
		t.Errorf("first binary should be fail, got %v", results[0].Status)
	}
	if results[1].Status != StatusPass {
		t.Errorf("second binary should be pass, got %v", results[1].Status)
	}
}

func TestParseChromiumLddOutput_NoneCached(t *testing.T) {
	results := parseChromiumLddOutput("")
	if len(results) != 1 || results[0].Status != StatusWarn || results[0].Name != chromiumWarnName {
		t.Fatalf("results = %+v, want single warn", results)
	}
}

func TestCheckChromiumLdd_NoGlobsConfigured(t *testing.T) {
	m := &Manifest{}
	results := checkChromiumLdd(context.Background(), &fakeExecutor{}, m)
	if results != nil {
		t.Fatalf("expected nil when no globs configured, got %+v", results)
	}
}
