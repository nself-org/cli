package plugin

// error_not_zero_test.go — pins the two sites in this package that used to
// convert a failure into a plausible-looking value.
//
// Purpose: a failed operation must not come back as a confident wrong answer.
//          `health` reported every unreachable plugin as a plain "not healthy",
//          and `getSchemaVersion` reported every psql failure as "version 0".
//          Both are values a caller acts on, so both hid their real cause.
// Inputs:  a closed httptest listener (guaranteed-refused port); an already
//          cancelled context.
// Outputs: assertions that the error is returned, not flattened.
// Constraints: no docker, no network, no daemon — both failures are produced
//              locally and deterministically.

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestHealth_UnreachablePortReturnsTheTransportError asserts that a plugin we
// cannot connect to yields the underlying dial error, not (false, nil).
//
// The caller (waitForPluginHealth) stores this as lastErr and puts it in the
// message it returns when the deadline expires. With (false, nil) that message
// was always the generic "timed out waiting for plugin to become healthy",
// which says nothing about whether the port was refused, the plugin crashed,
// or the request never left the process.
func TestHealth_UnreachablePortReturnsTheTransportError(t *testing.T) {
	// Start a server purely to reserve a port, then close it. Nothing is
	// listening on that port afterwards, so the dial is refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	srv.Close()

	ok, err := health(context.Background(), "ghost", port)
	if ok {
		t.Fatal("health reported ok against a closed port")
	}
	if err == nil {
		t.Fatal("health returned (false, nil) for an unreachable plugin; " +
			"the transport error must be returned so the caller can report why")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should name the plugin, got %v", err)
	}
}

// TestHealth_CancelledContextReturnsTheError covers the second failure that
// used to read as "not healthy": the request never being made at all.
func TestHealth_CancelledContextReturnsTheError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok, err := health(ctx, "cancelled", port)
	if ok {
		t.Fatal("health reported ok on a cancelled context")
	}
	if err == nil {
		t.Fatal("health swallowed a cancelled context into a plain 'not healthy'")
	}
}

// TestHealth_LiveServerStillWorks guards the happy path and the honest
// unhealthy path — a reachable plugin answering non-200 is genuinely not
// healthy, and that is (false, nil) with no error, as before.
func TestHealth_LiveServerStillWorks(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	ok, err := health(context.Background(), "live", port)
	if err != nil || !ok {
		t.Fatalf("healthy plugin: got (%v, %v), want (true, nil)", ok, err)
	}

	status = http.StatusServiceUnavailable
	ok, err = health(context.Background(), "live", port)
	if err != nil {
		t.Fatalf("a reachable plugin answering 503 is not an error, got %v", err)
	}
	if ok {
		t.Error("503 reported as healthy")
	}
}

// TestGetSchemaVersion_PSQLFailureIsNotVersionZero asserts that a psql failure
// propagates instead of being reported as "this plugin has applied nothing".
//
// An already-cancelled context makes exec.CommandContext fail before it starts
// the process, so this reproduces a psql failure without docker, a daemon, or
// a container. The old code answered (0, nil) here, which sends
// createPluginSchema on to re-run the entire DDL sequence and fail later
// complaining about CREATE ROLE rather than about the connection.
func TestGetSchemaVersion_PSQLFailureIsNotVersionZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &config.Config{ProjectName: "nselftest"}
	cfg.Postgres.User = "postgres"
	cfg.Postgres.DB = "nself"

	v, err := getSchemaVersion(ctx, cfg, "ai")
	if err == nil {
		t.Fatalf("getSchemaVersion returned (%d, nil) when psql could not run; "+
			"a failed query must not be reported as schema version 0", v)
	}
	if v != 0 {
		t.Errorf("version should stay at the zero value alongside the error, got %d", v)
	}
	if !strings.Contains(err.Error(), "ai") {
		t.Errorf("error should name the plugin, got %v", err)
	}
}
