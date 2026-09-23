package verify

// verify_sbom_test.go — regression coverage for the licensed-plugin install
// stall fixed alongside this file: VerifySBOM used a 30s client timeout and
// treated any transport error as a hard install failure, so a slow or
// non-responding lookup (measured at ~15-20s per licensed plugin in
// golden-path run 35813011314) either stalled every install or, worse, could
// turn into an outright failure. These tests pin the fail-open behavior and
// the tightened timeout so neither regresses.
//
// Purpose: verify VerifySBOM proceeds quickly (never blocks on the old 30s
// budget) for a hanging server, a definitive 404, and a skipped check, while
// still hard-failing on a malformed SBOM actually served with HTTP 200 — the
// one case that indicates tampering/corruption rather than mere absence.
// Inputs: httptest servers simulating each network condition.
// Outputs: pass/fail on VerifySBOM's returned error and timing bound.
// Constraints: overrides the package-level sbomBaseURL test seam; must
// restore it so tests stay order-independent.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// withSBOMBaseURL points VerifySBOM at a test server for the duration of the
// test and restores the real GitHub base URL on cleanup.
func withSBOMBaseURL(t *testing.T, url string) {
	t.Helper()
	orig := sbomBaseURL
	sbomBaseURL = url
	t.Cleanup(func() { sbomBaseURL = orig })
}

// TestVerifySBOM_HangingServerFailsOpenQuickly is the direct regression test
// for the stall: a server that never responds must not be allowed to hold up
// the install for anywhere near the old 30s budget, and must not turn into an
// install-blocking error — checksum and signature verification already ran
// before this step, so an unreachable, purely-advisory SBOM probe proceeds.
func TestVerifySBOM_HangingServerFailsOpenQuickly(t *testing.T) {
	block := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // never respond within the test
	}))
	// srv.Close() waits for the in-flight handler to return, so the block
	// channel must be closed (unblocking the handler) BEFORE srv.Close() runs
	// — a single deferred func in this order, not two separate defers/cleanups,
	// which would deadlock the test on shutdown regardless of VerifySBOM's own
	// (correct) 5s fail-open behavior.
	defer func() {
		close(block)
		srv.Close()
	}()
	withSBOMBaseURL(t, srv.URL)

	start := time.Now()
	err := VerifySBOM(context.Background(), "google", "1.1.2", SBOMCheckOptions{Version: "1.1.2"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("VerifySBOM on a hanging server must fail open (nil), got: %v", err)
	}
	if elapsed > sbomProbeTimeout+2*time.Second {
		t.Fatalf("VerifySBOM took %s, want well under the old 30s budget (probe timeout %s)", elapsed, sbomProbeTimeout)
	}
}

// TestVerifySBOM_404IsNonFatal pins the pre-existing "no SBOM published"
// behavior (older/non-GitHub-hosted releases) as still non-fatal.
func TestVerifySBOM_404IsNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	withSBOMBaseURL(t, srv.URL)

	if err := VerifySBOM(context.Background(), "cron", "1.2.5", SBOMCheckOptions{Version: "1.2.5"}); err != nil {
		t.Fatalf("VerifySBOM on 404 must return nil, got: %v", err)
	}
}

// TestVerifySBOM_ServerErrorIsNonFatal covers the broadened fail-open case
// (403/5xx): equally "could not obtain a valid SBOM from this source",
// not evidence of tampering, so it must not block the install either.
func TestVerifySBOM_ServerErrorIsNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	withSBOMBaseURL(t, srv.URL)

	if err := VerifySBOM(context.Background(), "google", "1.1.2", SBOMCheckOptions{Version: "1.1.2"}); err != nil {
		t.Fatalf("VerifySBOM on HTTP 500 must return nil, got: %v", err)
	}
}

// TestVerifySBOM_ValidSBOMStillVerified ensures a real, present SBOM is still
// fetched and schema-checked — the fail-open changes above must not weaken
// this path when an artifact genuinely exists.
func TestVerifySBOM_ValidSBOMStillVerified(t *testing.T) {
	valid := `{"bomFormat":"CycloneDX","specVersion":"1.5","version":1,"components":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(valid))
	}))
	defer srv.Close()
	withSBOMBaseURL(t, srv.URL)

	if err := VerifySBOM(context.Background(), "notify", "1.2.5", SBOMCheckOptions{Version: "1.2.5"}); err != nil {
		t.Fatalf("VerifySBOM on a valid CycloneDX SBOM must pass, got: %v", err)
	}
}

// TestVerifySBOM_MalformedSBOMStillFails ensures a served-but-corrupt/tampered
// SBOM (HTTP 200, invalid content) still hard-fails — the one case that
// actually indicates something is wrong, as opposed to the source simply not
// having one.
func TestVerifySBOM_MalformedSBOMStillFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"bomFormat":"NotCycloneDX"}`))
	}))
	defer srv.Close()
	withSBOMBaseURL(t, srv.URL)

	if err := VerifySBOM(context.Background(), "google", "1.1.2", SBOMCheckOptions{Version: "1.1.2"}); err == nil {
		t.Fatal("VerifySBOM on a malformed 200 response must return an error, got nil")
	}
}

// TestVerifySBOM_SkipCheckUsesCustomReason confirms the licensed-plugin skip
// path (SkipReason set by the installer for paid plugins) still short-
// circuits without making any network call at all — the actual fix for the
// golden-path stall, which the SkipCheck=true branch existed for before this
// change but is now exercised with a non-default reason.
func TestVerifySBOM_SkipCheckUsesCustomReason(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	withSBOMBaseURL(t, srv.URL)

	err := VerifySBOM(context.Background(), "google", "1.1.2", SBOMCheckOptions{
		SkipCheck:  true,
		Version:    "1.1.2",
		SkipReason: "licensed plugin — SBOM (if any) is served via ping.nself.org, not GitHub Releases",
	})
	if err != nil {
		t.Fatalf("VerifySBOM with SkipCheck must return nil, got: %v", err)
	}
	if called {
		t.Fatal("VerifySBOM with SkipCheck must not make any network call")
	}
}
