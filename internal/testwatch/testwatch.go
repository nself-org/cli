// Package testwatch gives a test package a hard wall-clock limit shorter than
// go test's default ten minutes, so a lock that is never released fails the
// package in about a minute with a goroutine dump instead of freezing a CI
// host for fifteen (cam handoff 2026-10-13, the bundle operation-lock hang).
//
// Use it from TestMain in packages that wait on file locks:
//
//	func TestMain(m *testing.M) { testwatch.Main(m, 90*time.Second) }
//
// Inputs: the package's *testing.M and the limit. Output: crashes the test binary (exit 2) with all
// goroutine stacks in the panic message when the limit passes; otherwise the
// process exits with m.Run's code.
// Constraints: stdlib only; the limit must be well above the package's normal run time.
package testwatch

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// Main runs the tests under a watchdog. It returns after m.Run; since Go 1.15
// the test wrapper then exits with m.Run's result, so no os.Exit is needed here.
func Main(m *testing.M, limit time.Duration) {
	timer := time.AfterFunc(limit, func() { fail(limit) })
	defer timer.Stop()
	m.Run()
}

// fail dumps every goroutine and crashes the test binary (exit status 2).
func fail(limit time.Duration) {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	panic(fmt.Sprintf("testwatch: the package did not finish within %s; a lock or channel is probably never released.\n%s", limit, buf))
}
