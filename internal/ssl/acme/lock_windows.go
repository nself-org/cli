//go:build windows

package acme

import "time"

// Lock is a no-op on Windows: the ACME path targets Linux (Windows runs the CLI under WSL2).
func Lock(string, time.Duration) (func(), error) { return func() {}, nil }
