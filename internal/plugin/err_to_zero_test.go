//go:build !windows

package plugin

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestGetSchemaVersion_PsqlFailureIsAnError guards D-0061: a failed psql query
// used to read as "schema version 0", so a broken database looked like a fresh
// one and the caller re-ran schema provisioning against it.
func TestGetSchemaVersion_PsqlFailureIsAnError(t *testing.T) {
	dir := t.TempDir()
	stub := "#!/bin/sh\necho 'Error response from daemon: No such container' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	v, err := getSchemaVersion(context.Background(), &config.Config{ProjectName: "demo"}, "cron")
	if err == nil {
		t.Fatalf("getSchemaVersion = %d, nil; want an error when psql fails", v)
	}
}

// TestHealth_UnreachablePluginReturnsDialError guards D-0061: an unreachable
// plugin used to return (false, nil), so the start loop could only report a
// generic "timed out" instead of the dial error it had actually seen.
func TestHealth_UnreachablePluginReturnsDialError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // nothing listens on port any more

	ok, err := health(context.Background(), "gone", port)
	if ok {
		t.Fatal("health reported ok for an unreachable plugin")
	}
	if err == nil {
		t.Fatal("health returned a nil error for an unreachable plugin; want the dial error")
	}
}
